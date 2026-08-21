"use strict";

importScripts("core/media.js", "core/store.js", "core/cookies.js", "core/launcher.js");

const requestMeta = new Map();
const tabQueues = new Map();
const DEFAULT_OPEN_MODE = "popup";
const POPUP_PATH = "popup/popup.html";
const MEDIA_SNIFFER_CONFIG_KEY = "mediaSnifferConfig";
let currentOpenMode = DEFAULT_OPEN_MODE;
let currentMediaSnifferConfig = DownKitMedia.normalizeSnifferConfig();

function hasEnabledMediaType(config) {
  return Boolean(config.detectHls || config.detectDash || config.detectMp4);
}

const mediaSnifferConfigReady = chrome.storage.local.get(MEDIA_SNIFFER_CONFIG_KEY)
  .then(result => {
    const stored = DownKitMedia.normalizeSnifferConfig(result[MEDIA_SNIFFER_CONFIG_KEY]);
    currentMediaSnifferConfig = hasEnabledMediaType(stored)
      ? stored
      : DownKitMedia.normalizeSnifferConfig();
    return currentMediaSnifferConfig;
  })
  .catch(error => {
    console.warn("[media-sniffer] 读取配置失败，使用默认检测范围", error);
    return currentMediaSnifferConfig;
  });

function normalizeOpenMode(value) {
  return value === "sidePanel" && chrome.sidePanel ? "sidePanel" : DEFAULT_OPEN_MODE;
}

async function applyOpenMode(value) {
  const mode = normalizeOpenMode(value);
  await chrome.action.setPopup({ popup: mode === "popup" ? POPUP_PATH : "" });
  if (chrome.sidePanel) {
    await chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: mode === "sidePanel" });
  }
  return mode;
}

const openModeReady = chrome.storage.local.get("openMode")
  .then(result => applyOpenMode(result.openMode))
  .catch(error => {
    console.warn(error);
    return applyOpenMode(DEFAULT_OPEN_MODE);
  })
  .then(mode => {
    currentOpenMode = mode;
    return mode;
  });

function headersToContext(headers) {
  const context = {};
  const requestHeaders = {};
  for (const header of headers || []) {
    const name = String(header.name || "").toLowerCase();
    const value = String(header.value || "");
    if (name !== "cookie") requestHeaders[name] = value;
    if (name === "referer") context.referer = value;
    if (name === "origin") context.origin = value;
    if (name === "user-agent") context.userAgent = value;
  }
  context.requestHeaders = DownKitMedia.sanitizeRequestHeaders(requestHeaders);
  return context;
}

let bridgeSession = null;

function requestNativeBridge() {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendNativeMessage("com.downkit.bridge", { command: "ensure_bridge" }, response => {
      const error = chrome.runtime.lastError;
      if (error) {
        reject(new Error(`无法连接 DownKit Bridge：${error.message}`));
        return;
      }
      if (!response || !response.ok || !response.baseUrl || !response.token) {
        reject(new Error(response && response.error || "DownKit Bridge 返回无效结果"));
        return;
      }
      resolve(response);
    });
  });
}

async function ensureBridge(force) {
  if (!force && bridgeSession && Date.now() - bridgeSession.checkedAt < 30000) return bridgeSession;
  const response = await requestNativeBridge();
  bridgeSession = { ...response, checkedAt: Date.now() };
  return bridgeSession;
}

async function bridgeFetch(path, options = {}, retry = true) {
  const bridge = await ensureBridge(false);
  const headers = {
    ...(options.body ? { "Content-Type": "application/json" } : {}),
    "X-DownKit-Token": bridge.token,
    ...(options.headers || {})
  };
  const response = await fetch(`${bridge.baseUrl}${path}`, { ...options, headers });
  if (response.status === 401 && retry) {
    bridgeSession = null;
    return bridgeFetch(path, options, false);
  }
  const result = await response.json().catch(() => ({}));
  if (!response.ok || result.ok === false) {
    throw new Error(result.error || `Bridge 请求失败：HTTP ${response.status}`);
  }
  return result;
}

function delay(milliseconds) {
  return new Promise(resolve => setTimeout(resolve, milliseconds));
}

async function restartBridge() {
  const previousPID = bridgeSession && bridgeSession.pid;
  await bridgeFetch("/v1/restart", { method: "POST" }, false);
  bridgeSession = null;
  await delay(300);
  const deadline = Date.now() + 10000;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const session = await requestNativeBridge();
      if (!previousPID || session.pid !== previousPID) {
        bridgeSession = { ...session, checkedAt: Date.now() };
        return bridgeSession;
      }
    } catch (error) {
      lastError = error;
    }
    await delay(250);
  }
  throw new Error(lastError && lastError.message || "Bridge 重启超时");
}

async function buildBridgeTask(resource, page, quality, playlist) {
  const resolveFromPage = resource.kind === "resolved-media-page" || resource.kind === "dash";
  const mediaURL = resolveFromPage ? String(page && page.url || resource.url) : resource.url;
  const pageURL = String(page && page.url || "");
  const cookieStoreId = await DownKitCookies.storeIDForTab(chrome.cookies, resource.tabId);
  const [mediaCookies, pageCookies] = await Promise.all([
    DownKitCookies.forURL(chrome.cookies, mediaURL, resource.tabId, resource.frameId, cookieStoreId),
    pageURL && pageURL !== mediaURL ? DownKitCookies.forURL(chrome.cookies, pageURL, resource.tabId, 0, cookieStoreId) : Promise.resolve([])
  ]);
  return DownKitLauncher.buildTask(
    resource,
    page,
    quality || "",
    mediaCookies,
    pageCookies.length ? pageCookies : (resolveFromPage ? mediaCookies : []),
    playlist || "ask",
    cookieStoreId
  );
}

async function refreshedJobCredentials(job) {
  const sourceURL = String(job && job.sourceUrl || "");
  const pageURL = String(job && job.pageUrl || "");
  const cookieStoreId = String(job && job.cookieStoreId || "");
  const tabId = pageURL && Number.isInteger(job && job.sourceTabId) && job.sourceTabId >= 0
    ? job.sourceTabId
    : undefined;
  const frameId = Number.isInteger(job && job.sourceFrameId) && job.sourceFrameId >= 0 ? job.sourceFrameId : undefined;
  const [mediaCookies, pageCookies] = await Promise.all([
    DownKitCookies.forURL(chrome.cookies, sourceURL, tabId, frameId, cookieStoreId),
    pageURL && pageURL !== sourceURL
      ? DownKitCookies.forURL(chrome.cookies, pageURL, tabId, 0, cookieStoreId)
      : Promise.resolve([])
  ]);
  return {
    userAgent: navigator.userAgent,
    // An omitted cookie collection means that Chromium could not provide a
    // fresher value (for example, the source tab was closed). The Bridge then
    // keeps the still-memory-only credentials from the paused run.
    ...(mediaCookies.length ? { mediaCookies } : {}),
    ...(pageCookies.length ? { pageCookies } : {})
  };
}

async function probeBridgePlaylist(resource, page, quality) {
  const task = await buildBridgeTask(resource, page, quality, "single");
  return bridgeFetch("/v1/playlist/probe", {
    method: "POST",
    body: JSON.stringify(task)
  });
}

async function submitBridgeTask(resource, page, quality, playlist) {
  const task = await buildBridgeTask(resource, page, quality, playlist || "single");
  return bridgeFetch("/v1/tasks", {
    method: "POST",
    body: JSON.stringify(task)
  });
}

function nativeHostTool(health) {
  return {
    name: "native-host",
    displayName: "Native Messaging Host",
    kind: "runtime",
    description: "由浏览器启动并连接 DownKit Bridge。",
    platforms: ["windows", "linux", "darwin"],
    delivery: "bundled",
    required: true,
    capabilities: ["bridge.launch"],
    sortOrder: 10,
    config: { schema: [], values: {}, defaultExpanded: false },
    health
  };
}

function mediaSnifferTool() {
  const config = DownKitMedia.normalizeSnifferConfig(currentMediaSnifferConfig);
  const enabledLabels = [
    config.detectHls && "HLS",
    config.detectDash && "DASH",
    config.detectMp4 && "MP4"
  ].filter(Boolean);
  return {
    name: "media-sniffer",
    displayName: "媒体嗅探器",
    kind: "capability",
    description: "监听当前页面的媒体请求，并归并 HLS、DASH、MP4 与分离媒体轨。",
    platforms: ["chrome", "edge"],
    delivery: "extension",
    required: true,
    capabilities: ["media.detect", "media.group", "cookie.read-on-demand"],
    sortOrder: 30,
    config: {
      scope: "extension",
      schema: [
        { key: "detectHls", label: "HLS（m3u8）", type: "boolean", description: "检测 HLS 主清单和媒体清单。" },
        { key: "detectDash", label: "DASH / 分离媒体轨", type: "boolean", description: "检测 mpd、m4s 及页面级音视频轨。" },
        { key: "detectMp4", label: "MP4 直链", type: "boolean", description: "检测 mp4 URL 或 video/mp4 响应。" }
      ],
      values: config,
      defaultExpanded: false
    },
    health: {
      status: "ready",
      ok: true,
      summary: `正在检测 ${enabledLabels.join("、")}`,
      detail: "仅在点击下载时读取匹配 URL 的 Cookie"
    }
  };
}

function offlineBridgeTool(detail) {
  return {
    name: "bridge",
    displayName: "DownKit Bridge",
    kind: "runtime",
    description: "连接浏览器与本地下载引擎的服务。",
    platforms: ["windows", "linux", "darwin"],
    delivery: "bundled",
    required: true,
    capabilities: ["task.submit", "job.manage", "config.persist"],
    sortOrder: 20,
    config: { schema: [], values: {}, defaultExpanded: false },
    health: {
      status: "error",
      ok: false,
      summary: "Bridge 不可访问",
      detail: `Native Host 连接失败，扩展无法取得 Bridge 会话 token。${detail ? ` ${String(detail)}` : ""}`
    }
  };
}

function legacyToolSnapshot(environment, config) {
  const executableHealth = (status, missingSummary) => status && status.available ? {
    status: "ready", ok: true, summary: "组件就绪", version: status.version || "", path: status.path || ""
  } : {
    status: "missing", ok: false, summary: missingSummary, detail: status && status.error || "未找到"
  };
  return [
    {
      name: "bridge", displayName: "DownKit Bridge", kind: "runtime",
      description: "连接浏览器与本地下载引擎的服务。", platforms: ["windows", "linux", "darwin"],
      delivery: "bundled", required: true, sortOrder: 20,
      config: { schema: [{ key: "address", label: "监听地址", type: "string", readOnly: true }], values: { address: "自动发现" }, defaultExpanded: false },
      health: { status: "ready", ok: true, summary: "Bridge 在线", version: environment.version || "", detail: `PID ${environment.pid || "?"} · ${environment.platform || ""}`, path: environment.executable || "" }
    },
    {
	  name: "network-proxy", displayName: "网络代理", kind: "infra",
	  description: "统一配置 Bridge 发起的外网请求；本机控制通信保持直连。", platforms: ["windows", "linux", "darwin"],
	  delivery: "bundled", required: false, sortOrder: 25,
	  config: {
		toggle: { key: "proxyEnabled", label: "启用代理", description: "关闭后保留代理地址，Bridge 外网请求改为直连。" },
		schema: [
		  { key: "proxyHost", label: "代理主机", type: "string", placeholder: "例如 127.0.0.1", description: "只填写域名或 IP，不要包含 http:// 或端口。" },
		  { key: "proxyPort", label: "代理端口", type: "integer", min: 1, max: 65535, description: "主机和端口都留空表示直连。" }
		],
		values: { proxyEnabled: config.proxyEnabled ?? Boolean(config.proxyHost), proxyHost: config.proxyHost || "", proxyPort: config.proxyPort || "" }, defaultExpanded: false
	  },
	  health: { status: (config.proxyEnabled ?? Boolean(config.proxyHost)) ? "checking" : "disabled", ok: true, summary: (config.proxyEnabled ?? Boolean(config.proxyHost)) ? "等待代理检测" : "未启用 · Bridge 外网直连" }
	},
	{
      name: "go-downloader", displayName: "Go 下载引擎", kind: "capability",
      description: "负责清单解析、分片下载、解密和断点续传。", platforms: ["windows", "linux", "darwin", "android", "ios"],
      delivery: "bundled", required: true, sortOrder: 30,
      actions: [{ id: "open-output", label: "打开下载目录", description: "在文件管理器中打开当前下载目录。" }],
      config: {
        schema: [
          { key: "outputDir", label: "下载目录", type: "directory" },
          { key: "concurrent", label: "下载并发上限", type: "integer", min: 1, max: 64, description: "统一控制 HLS 分片、MP4 Range 和 yt-dlp 分片的最大并发数。" },
          { key: "quality", label: "默认清晰度", type: "select", options: [
            { value: "", label: "有多个时询问" },
            { value: "best", label: "最高" },
            { value: "1080", label: "1080p" },
            { value: "720", label: "720p" },
            { value: "480", label: "480p" }
          ] }
        ],
		values: { outputDir: config.outputDir || "", concurrent: config.concurrent || 12, quality: config.quality || "" },
        defaultExpanded: false
      },
      health: { status: "ready", ok: true, summary: "下载引擎就绪" }
    },
    {
      name: "ffmpeg", displayName: "FFmpeg Slim", kind: "dependency",
      description: "无损封装 HLS、TS、fMP4 及分离音视频轨。", platforms: ["windows", "linux", "darwin"],
      delivery: "bundled-sidecar", required: false, sortOrder: 40,
      config: { schema: [{ key: "ffmpegPath", label: "程序路径", type: "file", placeholder: "请输入程序完整路径", description: "自动检测失败时，请指定可信的程序文件。" }], values: { ffmpegPath: config.ffmpegPath || "" }, defaultExpanded: false },
      health: executableHealth(environment.ffmpeg, "组件未找到")
    },
    {
      name: "yt-dlp", displayName: "yt-dlp", kind: "dependency",
      description: "按需解析来源页面和通用媒体站点。", platforms: ["windows", "linux", "darwin"],
      delivery: "on-demand", required: false, sortOrder: 50,
      config: { schema: [{ key: "ytDlpPath", label: "程序路径", type: "file", placeholder: "请输入程序完整路径", description: "自动检测失败时，请指定可信的程序文件。" }], values: { ytDlpPath: config.ytDlpPath || "" }, defaultExpanded: false },
      health: executableHealth(environment.ytDlp, "组件未找到")
    }
  ];
}

async function bridgeToolsSnapshot() {
  await mediaSnifferConfigReady;
  let session;
  try {
    session = await ensureBridge(false);
  } catch (error) {
    const detail = error.message || String(error);
    return {
      ok: true,
      version: 1,
      tools: [
        nativeHostTool({ status: "error", ok: false, summary: "Native Host 不可用", detail }),
        mediaSnifferTool(),
        offlineBridgeTool(detail)
      ]
    };
  }

  const native = nativeHostTool({
    status: "ready", ok: true, summary: "Native Host 已连接",
    detail: "浏览器原生通道正常"
  });
  const sniffer = mediaSnifferTool();
  try {
    const result = await bridgeFetch("/v1/tools");
    return { ...result, ok: true, tools: [native, sniffer, ...(result.tools || [])] };
  } catch (error) {
    try {
      const [environmentResult, configResult] = await Promise.all([
        bridgeFetch("/v1/environment"), bridgeFetch("/v1/config")
      ]);
      return {
        ok: true,
        version: 1,
        legacy: true,
        tools: [native, sniffer, ...legacyToolSnapshot(environmentResult.environment || {}, configResult.config || {})]
      };
    } catch (_) {
      return {
        ok: true,
        version: 1,
        tools: [native, sniffer, offlineBridgeTool(error.message || String(error))]
      };
    }
  }
}

function responseContentType(headers) {
  const found = (headers || []).find(header => String(header.name || "").toLowerCase() === "content-type");
  return found && found.value || "";
}

function enqueue(tabId, work) {
  const previous = tabQueues.get(tabId) || Promise.resolve();
  const current = previous.catch(() => {}).then(work);
  let queued;
  queued = current.finally(() => {
    if (tabQueues.get(tabId) === queued) tabQueues.delete(tabId);
  });
  tabQueues.set(tabId, queued);
  return queued;
}

async function record(input) {
  if (!Number.isInteger(input.tabId) || input.tabId < 0) return;
  await mediaSnifferConfigReady;
  const resource = DownKitMedia.makeResource(input, currentMediaSnifferConfig);
  if (!resource) return;
  const items = await enqueue(resource.tabId, () => DownKitStore.put(resource));
  const visibleItems = items.filter(item => DownKitMedia.configAllowsKind(item.kind, currentMediaSnifferConfig));
  await chrome.action.setBadgeBackgroundColor({ tabId: resource.tabId, color: "#2563eb" });
  await chrome.action.setBadgeText({ tabId: resource.tabId, text: String(Math.min(visibleItems.length, 99)) });
}

async function refreshMediaBadges() {
  const tabs = await chrome.tabs.query({});
  await Promise.all(tabs.filter(tab => Number.isInteger(tab.id)).map(async tab => {
    try {
      const items = await DownKitStore.get(tab.id);
      const visibleItems = items.filter(item => DownKitMedia.configAllowsKind(item.kind, currentMediaSnifferConfig));
      await chrome.action.setBadgeText({
        tabId: tab.id,
        text: visibleItems.length ? String(Math.min(visibleItems.length, 99)) : ""
      });
    } catch (error) {
      console.warn(`[media-sniffer] 刷新标签页 ${tab.id} 计数失败`, error);
    }
  }));
}

chrome.webRequest.onBeforeRequest.addListener(details => {
  const current = {
    url: details.url,
    tabId: details.tabId,
    frameId: details.frameId,
    initiator: details.initiator || "",
    detectedAt: Date.now()
  };
  if (!DownKitMedia.classify(current, currentMediaSnifferConfig)) return;
  requestMeta.set(details.requestId, current);
  record(current).catch(console.warn);
}, { urls: ["<all_urls>"] });

chrome.webRequest.onBeforeSendHeaders.addListener(details => {
  const current = requestMeta.get(details.requestId);
  if (!current) return;
  Object.assign(current, headersToContext(details.requestHeaders));
  requestMeta.set(details.requestId, current);
  record(current).catch(console.warn);
}, { urls: ["<all_urls>"] }, ["requestHeaders", "extraHeaders"]);

chrome.webRequest.onHeadersReceived.addListener(details => {
  const current = requestMeta.get(details.requestId) || {
    url: details.url,
    tabId: details.tabId,
    frameId: details.frameId,
    initiator: details.initiator || "",
    detectedAt: Date.now()
  };
  Object.assign(current, {
    contentType: responseContentType(details.responseHeaders),
    statusCode: details.statusCode
  });
  requestMeta.set(details.requestId, current);
  record(current).catch(console.warn);
}, { urls: ["<all_urls>"] }, ["responseHeaders", "extraHeaders"]);

function forget(details) { requestMeta.delete(details.requestId); }
chrome.webRequest.onCompleted.addListener(forget, { urls: ["<all_urls>"] });
chrome.webRequest.onErrorOccurred.addListener(forget, { urls: ["<all_urls>"] });

chrome.tabs.onRemoved.addListener(tabId => {
  DownKitStore.clear(tabId).catch(() => {});
  tabQueues.delete(tabId);
});

chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
  if (!changeInfo.url) return;
  DownKitStore.clear(tabId).catch(() => {});
  chrome.action.setBadgeText({ tabId, text: "" }).catch(() => {});
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (!message || !message.action) return;

  if (message.action === "ui.open-mode.get") {
    openModeReady
      .then(() => sendResponse({ ok: true, mode: currentOpenMode, supportsSidePanel: Boolean(chrome.sidePanel) }), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "ui.open-mode.set") {
    openModeReady
      .then(() => applyOpenMode(message.mode))
      .then(mode => chrome.storage.local.set({ openMode: mode }).then(() => mode))
      .then(mode => {
        currentOpenMode = mode;
        sendResponse({ ok: true, mode, supportsSidePanel: Boolean(chrome.sidePanel) });
      }, error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "media.detected") {
    const tabId = sender.tab && sender.tab.id;
    record({
      url: message.url,
      contentType: message.contentType || "",
      tabId,
      initiator: sender.origin || "",
      referer: sender.tab && sender.tab.url || "",
      userAgent: message.userAgent || "",
      detectedAt: Date.now()
    }).then(() => sendResponse({ ok: true }), error => sendResponse({ ok: false, error: String(error) }));
    return true;
  }

  if (message.action === "media.list") {
    Promise.all([mediaSnifferConfigReady, DownKitStore.get(message.tabId), chrome.tabs.get(message.tabId)])
      .then(([, items, tab]) => {
        const page = { url: tab.url || "", title: tab.title || "", userAgent: navigator.userAgent };
        sendResponse({ ok: true, page, items: DownKitMedia.displayList(items, page, currentMediaSnifferConfig) });
      }, error => sendResponse({ ok: false, error: String(error) }));
    return true;
  }

  if (message.action === "media.clear") {
    DownKitStore.clear(message.tabId)
      .then(() => chrome.action.setBadgeText({ tabId: message.tabId, text: "" }))
      .then(() => sendResponse({ ok: true }), error => sendResponse({ ok: false, error: String(error) }));
    return true;
  }

  if (message.action === "media.download") {
    submitBridgeTask(message.resource, message.page, message.quality || "", message.playlist || "single")
      .then(result => sendResponse({ ok: true, taskId: result.taskId }), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "media.playlist.probe") {
    probeBridgePlaylist(message.resource, message.page, message.quality || "")
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.environment") {
    Promise.all([bridgeFetch("/v1/environment"), bridgeFetch("/v1/config")])
      .then(([environment, config]) => sendResponse({
        ok: true,
        nativeHost: true,
        environment: environment.environment,
        config: config.config
      }), error => sendResponse({ ok: false, nativeHost: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.tools") {
    bridgeToolsSnapshot()
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.tool.action") {
    const tool = String(message.tool || "");
    const action = String(message.toolAction || "");
    if (tool === "bridge" && action === "restart") {
      restartBridge()
        .then(session => sendResponse({ ok: true, pid: session.pid }), error => sendResponse({ ok: false, error: error.message || String(error) }));
      return true;
    }
    if (tool === "go-downloader" && action === "open-output") {
      bridgeFetch("/v1/open-output", { method: "POST" })
        .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
      return true;
    }
    if (tool !== "yt-dlp" || action !== "install") {
      sendResponse({ ok: false, error: "不支持的 Tool 操作" });
      return;
    }
    bridgeFetch("/v1/tools/yt-dlp/install", { method: "POST" })
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.config.save") {
    bridgeFetch("/v1/config", { method: "PUT", body: JSON.stringify(message.config || {}) })
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "tool.config.save") {
    const tool = String(message.tool || "");
    if (tool !== "media-sniffer") {
      bridgeFetch("/v1/config", { method: "PUT", body: JSON.stringify(message.config || {}) })
        .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
      return true;
    }
    const next = DownKitMedia.normalizeSnifferConfig(message.config);
    if (!hasEnabledMediaType(next)) {
      sendResponse({ ok: false, error: "至少保留一种媒体检测类型" });
      return;
    }
    chrome.storage.local.set({ [MEDIA_SNIFFER_CONFIG_KEY]: next })
      .then(async () => {
        currentMediaSnifferConfig = next;
        requestMeta.clear();
        await refreshMediaBadges();
        return next;
      })
      .then(config => sendResponse({ ok: true, config }), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.jobs.list") {
    bridgeFetch("/v1/jobs")
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.job.retry") {
    refreshedJobCredentials(message.job || {})
      .then(credentials => bridgeFetch(`/v1/jobs/${encodeURIComponent(message.taskId || "")}/retry`, {
        method: "POST",
        body: JSON.stringify(credentials)
      }))
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.job.resume") {
    refreshedJobCredentials(message.job || {})
      .then(credentials => bridgeFetch(`/v1/jobs/${encodeURIComponent(message.taskId || "")}/resume`, {
        method: "POST",
        body: JSON.stringify(credentials)
      }))
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (["pause", "open", "reveal", "delete"].includes(message.action.replace("bridge.job.", "")) && message.action.startsWith("bridge.job.")) {
    const action = message.action.slice("bridge.job.".length);
    const body = action === "delete"
      ? { deleteFiles: Boolean(message.deleteFiles) }
      : action === "open" || action === "reveal"
        ? { outputPath: String(message.outputPath || "") }
        : null;
    bridgeFetch(`/v1/jobs/${encodeURIComponent(message.taskId || "")}/${action}`, {
      method: "POST",
      ...(body ? { body: JSON.stringify(body) } : {})
    })
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.jobs.clear") {
    bridgeFetch("/v1/jobs/clear", { method: "POST" })
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "bridge.output.open") {
    bridgeFetch("/v1/open-output", { method: "POST" })
      .then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }
});
