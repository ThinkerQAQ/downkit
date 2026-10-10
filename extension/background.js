"use strict";

importScripts("core/media.js", "core/store.js", "core/cookies.js", "core/launcher.js");

const requestMeta = new Map();
const tabQueues = new Map();
const DEFAULT_OPEN_MODE = "popup";
const POPUP_PATH = "popup/popup.html";
let currentOpenMode = DEFAULT_OPEN_MODE;
let currentMediaSnifferConfig = DownKitMedia.normalizeSnifferConfig();

function hasEnabledMediaType(config) {
  return Boolean(config.detectHls || config.detectDash || config.detectMp4);
}

// Data/config.toml is the sole source of persisted preferences.
// Only per-tab media detections live in browser session storage.
const preferencesReady = bridgeFetch("/v1/preferences").then(result => result.preferences);

const mediaSnifferConfigReady = preferencesReady
  .then(preferences => {
    currentMediaSnifferConfig = DownKitMedia.normalizeSnifferConfig(preferences);
    return currentMediaSnifferConfig;
  })
  .catch(error => {
    console.warn("[media-sniffer] TOML preferences unavailable", error);
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

const openModeReady = preferencesReady
  .then(preferences => applyOpenMode(preferences.openMode))
  .catch(error => {
    console.warn(error);
    return applyOpenMode(DEFAULT_OPEN_MODE);
  })
  .then(mode => { currentOpenMode = mode; return mode; });

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

async function buildBridgeTask(resource, page, quality, playlist, subtitles) {
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
    cookieStoreId,
    subtitles
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

async function submitBridgeTask(resource, page, quality, playlist, subtitles) {
  const task = await buildBridgeTask(resource, page, quality, playlist || "single", subtitles);
  return bridgeFetch("/v1/tasks", {
    method: "POST",
    body: JSON.stringify(task)
  });
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
    },
    health: {
      status: "ready",
      ok: true,
      summary: `正在检测 ${enabledLabels.join("、")}`,
      detail: "仅在点击下载时读取匹配 URL 的 Cookie"
    }
  };
}

// Native Messaging is the transport used to reach the Bridge, not a second
// independently configurable runtime. Show one local-service status while
// retaining distinct transport and server failure details for diagnostics.
function offlineBridgeTool(layer, detail) {
  return {
    name: "bridge",
    displayName: "本地服务",
    kind: "runtime",
    description: "浏览器原生连接与 DownKit 下载服务。",
    platforms: ["windows", "linux", "darwin"],
    delivery: "bundled",
    required: true,
    capabilities: ["bridge.launch", "task.submit", "job.manage", "config.persist"],
    sortOrder: 20,
    config: { schema: [], values: {} },
    health: {
      status: "error",
      ok: false,
      summary: "本地服务不可用",
      detail: `${layer}：${String(detail || "连接失败")}`
    }
  };
}

function localServiceTool(bridge) {
  const health = bridge.health || {};
  return {
    ...bridge,
    displayName: "本地服务",
    description: "浏览器原生连接与 DownKit 下载服务。",
    health: {
      ...health,
      summary: health.ok ? "浏览器与下载服务连接正常" : (health.summary || "本地服务不可用"),
      detail: ["Native Messaging 已连接", health.detail].filter(Boolean).join(" · ")
    }
  };
}

async function bridgeToolsSnapshot() {
  await mediaSnifferConfigReady;
  try {
    await ensureBridge(false);
  } catch (error) {
    return {
      ok: true,
      version: 1,
      tools: [mediaSnifferTool(), offlineBridgeTool("浏览器原生连接失败", error.message || String(error))]
    };
  }

  const sniffer = mediaSnifferTool();
  try {
    const result = await bridgeFetch("/v1/tools");
    return {
      ...result,
      ok: true,
      tools: [sniffer, ...(result.tools || []).map(tool =>
        tool.name === "bridge" ? localServiceTool(tool) : tool)]
    };
  } catch (error) {
    return {
      ok: true,
      version: 1,
      tools: [sniffer, offlineBridgeTool("下载服务连接失败", error.message || String(error))]
    };
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

  if (message.action === "ui.language.get") {
    bridgeFetch("/v1/preferences")
      .then(result => sendResponse({ ok: true, locale: result.preferences.uiLanguage }),
        error => sendResponse({ ok: false, error: String(error) }));
    return true;
  }
  if (message.action === "ui.language.set") {
    const locale = message.locale === "en" ? "en" : "zh-CN";
    bridgeFetch("/v1/preferences", { method: "PUT", body: JSON.stringify({ uiLanguage: locale }) })
      .then(() => sendResponse({ ok: true, locale }),
        error => sendResponse({ ok: false, error: String(error) }));
    return true;
  }

  if (message.action === "ui.active-tab") {
    chrome.tabs.query({})
      .then(tabs => {
        const candidates = tabs.filter(tab => /^https?:\/\//.test(tab.url || "") &&
          !String(tab.url).startsWith("http://127.0.0.1:17891/console/"));
        const current = candidates.find(tab => tab.active && tab.lastFocusedWindow) ||
          candidates.find(tab => tab.active) || candidates[0];
        sendResponse(current ? { ok: true, tab: { id: current.id, title: current.title, url: current.url } } :
          { ok: false, error: "No browser media tab available" });
      }, error => sendResponse({ ok: false, error: String(error) }));
    return true;
  }

  if (message.action === "ui.open-mode.get") {
    openModeReady
      .then(() => sendResponse({ ok: true, mode: currentOpenMode, supportsSidePanel: Boolean(chrome.sidePanel) }), error => sendResponse({ ok: false, error: error.message || String(error) }));
    return true;
  }

  if (message.action === "ui.open-mode.set") {
    openModeReady
      .then(() => applyOpenMode(message.mode))
      .then(mode => bridgeFetch("/v1/preferences", {
        method: "PUT", body: JSON.stringify({ openMode: mode })
      }).then(() => mode))
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
    submitBridgeTask(message.resource, message.page, message.quality || "", message.playlist || "single", message.subtitles)
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
    if (tool === "whisper.cpp" && action === "install-model") {
      bridgeFetch("/v1/tools/whisper/models/install", {
        method: "POST",
        body: JSON.stringify({ model: String(message.model || "") })
      }).then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
      return true;
    }
    if (tool === "llama.cpp" && action === "install-model") {
      bridgeFetch("/v1/tools/llama/models/install", {
        method: "POST",
        body: JSON.stringify({ model: String(message.model || ""), acceptLicense: Boolean(message.acceptLicense) })
      }).then(result => sendResponse(result), error => sendResponse({ ok: false, error: error.message || String(error) }));
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
    bridgeFetch("/v1/preferences", { method: "PUT", body: JSON.stringify(next) })
      .then(async () => {
        currentMediaSnifferConfig = next;
        requestMeta.clear();
        await refreshMediaBadges();
        sendResponse({ ok: true, config: next });
      }, error => sendResponse({ ok: false, error: error.message || String(error) }));
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
