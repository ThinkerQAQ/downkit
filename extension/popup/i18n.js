"use strict";

// Language resources are shared by popup, side panel and the local Web UI.
// Keys intentionally match existing Chinese messages so Bridge API contracts
// and user download-language preferences stay independent of UI locale.
(function (root) {
  const dictionary = {
    "DownKit 助手": "DownKit Assistant", "媒体下载助手": "Media downloader",
    "环境检测中": "Checking environment", "当前页面资源": "Resources on this page",
    "资源嗅探": "Detect", "下载任务": "Downloads", "工具与配置": "Tools & Settings",
    "清空": "Clear", "字幕": "Subtitles", "不下载字幕": "No subtitles",
    "原站中文（含自动字幕）": "Site Chinese (including auto captions)",
    "原站英文（含自动字幕）": "Site English (including auto captions)",
    "全部原站字幕": "All site subtitles",
    "原站优先，无字幕时本地识别": "Prefer site; transcribe locally if unavailable",
    "始终使用本地语音识别": "Always transcribe locally",
    "选择本次任务的字幕来源；本地识别依赖“工具与配置”中的 whisper.cpp。":
      "Choose subtitles for this task; local transcription requires whisper.cpp in Tools & Settings.",
    "视频语音": "Spoken language", "输出语言": "Output language",
    "字幕形式": "Subtitle format", "自动检测（推荐）": "Auto-detect (recommended)",
    "保持原语言": "Keep original language", "简体中文": "Simplified Chinese",
    "繁体中文": "Traditional Chinese", "中文 / 普通话": "Chinese / Mandarin",
    "英语": "English", "日语": "Japanese", "韩语": "Korean",
    "粤语": "Cantonese", "法语": "French", "德语": "German",
    "西班牙语": "Spanish", "俄语": "Russian", "葡萄牙语": "Portuguese",
    "意大利语": "Italian", "仅译文": "Translation only",
    "原文 + 译文（双语）": "Original + translation (bilingual)",
    "这是视频里说的语言，不是翻译目标语言；需要翻译时请明确选择。":
      "This is the language spoken in the video, not the target language.",
    "选择其他语言时会使用“工具与配置”中的 llama.cpp 在本机翻译。":
      "A different output language uses local llama.cpp translation.",
    "双语字幕保留原文，并在下一行加入译文。":
      "Bilingual subtitles show the translation below each original line.",
    "尚未检测到媒体。请先播放视频，再重新打开扩展。":
      "No media detected. Play a video, then reopen the extension.",
    "最近任务": "Recent tasks", "显示下载进度与任务操作。":
      "Monitor progress and manage downloads.", "总速度": "Total speed",
    "刷新": "Refresh", "清理": "Clear finished", "还没有下载任务。": "No downloads yet.",
    "复制": "Copy", "下载": "Download", "删除…": "Delete…",
    "打开": "Open", "位置": "Show folder",
    "打开方式": "Open mode", "弹出窗口（默认）": "Popup (default)",
    "浏览器侧边栏": "Browser side panel", "下次点击扩展图标时生效":
      "Applies next time you click the extension icon",
    "连接与运行时": "Connection & Runtime", "浏览器到本机服务的可信连接":
      "Trusted connection to the local service",
    "下载能力": "Download capabilities", "发现、解析和下载媒体":
      "Discover, resolve and download media",
    "外部组件": "Dependencies", "按需参与封装和页面解析":
      "Optional media processors and resolvers",
    "其他 Tool": "Other tools", "尚未分类的能力": "Uncategorized capabilities",
    "按连接、能力和依赖统一管理": "Manage runtime, capabilities and dependencies",
    "重新检测": "Check again", "正在读取 Tool Registry…": "Reading tool registry…",
    "设置": "Settings", "开启": "Enabled", "关闭": "Disabled",
    "未设置": "Not set", "检测中": "Checking", "编辑": "Edit",
    "保存": "Save", "取消": "Cancel",
    "可选": "Optional", "必选": "Required", "已保存，下次点击扩展图标时生效":
      "Saved. Applies next time the extension icon is clicked.",
    "当前浏览器不支持侧边栏": "Side panel unavailable in this browser",
    "正在保存…": "Saving…", "环境检测失败": "Environment check failed",
    "等待中": "Queued", "下载中": "Downloading", "已暂停": "Paused",
    "需重新授权": "Session required", "已完成": "Completed",
    "失败": "Failed", "解析中": "Resolving", "准备字幕": "Preparing subtitles",
    "提取音频": "Extracting audio", "识别字幕": "Transcribing",
    "加载翻译模型": "Loading translation model",
    "翻译字幕": "Translating", "生成字幕": "Finalizing subtitles",
    "下载文件": "Downloaded file", "工具": "Tools", "恢复": "Resume",
    "重试": "Retry", "暂停": "Pause", "删除下载任务？": "Delete download?",
    "当前页面包含": "This page contains", "个视频，请选择下载范围。":
      "videos. Choose what to download.",
    "检测到播放列表": "Playlist detected", "仅下载当前视频": "Current video only",
    "下载全部视频": "Download all videos", "仅删除记录": "Remove record only",
    "删除记录和文件": "Delete record and files", "停止并删除记录": "Stop and remove record",
    "清理已结束任务": "Clear finished tasks",
    "复制完整资源地址": "Copy media URL",
    "在浏览器标签页打开工作台": "Open workspace in browser tab",
    "打开工作台": "Open workspace", "界面语言": "Interface language",
    "画质": "Quality", "并发数": "Concurrent downloads",
    "下载目录": "Download directory", "代理": "Proxy",
    "代理开关": "Proxy enabled", "代理主机": "Proxy host", "代理端口": "Proxy port",
    "检查": "Check", "安装": "Install",
    "音频识别": "Speech recognition", "本地翻译": "Local translation",
    "启用": "Enable", "禁用": "Disable"
  };

  Object.assign(dictionary, {
  "本地": "Local",
  "连接浏览器与本地下载引擎的服务。": "Connects the browser to the local download engine.",
  "监听地址": "Listen address",
  "重启 Bridge": "Restart Bridge",
  "重新启动本地服务；存在未结束的下载任务时会拒绝操作。": "Restart the local service; active downloads prevent restart.",
  "Go 下载引擎": "Go downloader",
  "负责清单解析、分片下载、解密和断点续传。": "Resolves manifests, downloads and decrypts segments, and supports resume.",
  "下载并发上限": "Maximum concurrent downloads",
  "统一控制 HLS 分片、MP4 Range 和 yt-dlp 分片的最大并发数。": "Maximum workers for HLS, MP4 range and yt-dlp downloads.",
  "默认清晰度": "Default quality",
  "仅在媒体清单包含多个码率时生效。": "Used when multiple bitrates are available.",
  "有多个时询问": "Ask when multiple options exist",
  "最高": "Best",
  "打开下载目录": "Open download folder",
  "在文件管理器中打开当前下载目录。": "Open the current download directory in the file manager.",
  "网络代理": "Network proxy",
  "统一配置 Bridge 发起的外网请求；本机控制通信保持直连。": "Proxy external Bridge traffic; local control stays direct.",
  "启用代理": "Enable proxy",
  "关闭后保留代理地址，Bridge 外网请求改为直连。": "Keep proxy settings when disabled; external requests connect directly.",
  "只填写域名或 IP，不要包含 http:// 或端口。": "Enter only a hostname or IP, without scheme or port.",
  "主机和端口都留空表示直连。": "Leave host and port empty for a direct connection.",
  "程序路径": "Executable path",
  "请输入程序完整路径": "Enter the full executable path",
  "自动检测失败时，请指定可信的程序文件。": "Choose a trusted executable when auto-detection fails.",
  "下载组件": "Download component",
  "从 yt-dlp 官方 release 下载并校验 SHA-256。": "Download and verify SHA-256 from the official yt-dlp release.",
  "无损封装 HLS、TS、fMP4 及分离音视频轨。": "Mux HLS, TS, fMP4 and separate audio/video tracks without re-encoding.",
  "按需解析来源页面和通用媒体站点。": "Resolve source pages and supported media sites when needed.",
  "原站没有字幕时，在本机离线识别音频并生成 SRT。Windows 客户端随包提供，模型需由用户显式选择。": "Generate local SRT when the site provides no captions. The Windows client is bundled; models are optional.",
  "whisper-cli 程序路径": "whisper-cli executable",
  "GGML 模型路径": "GGML model path",
  "中文或多语言识别请勿选择名称以 .en 结尾的英语专用模型。": "For multilingual speech, do not select an English-only .en model.",
  "在本机运行 TranslateGemma，将原语言字幕翻译为目标语言。": "Run TranslateGemma locally to translate subtitles.",
  "llama-server 程序路径": "llama-server executable",
  "翻译 GGUF 模型路径": "Translation GGUF model path",
  "当前建议使用 TranslateGemma 4B Q4_K_M；模型许可需由用户自行确认。": "TranslateGemma 4B Q4_K_M is recommended; review the model licence.",
  "通过 gomobile 嵌入的 Go 下载引擎。": "Go downloader embedded through gomobile.",
  "系统媒体封装器": "System media muxer",
  "Android MediaExtractor/MediaMuxer 或 iOS 平台实现。": "Android MediaExtractor/MediaMuxer or the corresponding iOS implementation.",
  "系统封装失败时使用的原生回退；必须随 APK/AAB 发布。": "Native fallback used when system muxing fails; bundle it with APK/AAB.",
  "配置已保存。": "Configuration saved.",
  "正在检测当前页面…": "Detecting the current page…",
  "正在读取 Tool Registry…": "Loading tool registry…",
  "正在读取工具…": "Loading tools…",
  "未找到可用工具": "No tools available"
});

  Object.assign(dictionary, {
  "已取消下载。": "Download cancelled.",
  "已复制": "Copied",
  "当前页面": "Current page",
  "找不到当前标签页": "No active browser tab found",
  "正在检测播放列表…": "Detecting playlist…",
  "正在读取当前 URL 的 Cookie 并提交任务…": "Reading current page session and submitting task…",
  "浏览器未允许复制": "The browser did not permit copying",
  "翻译字幕时，请明确选择视频语音，不能使用自动检测": "Choose a spoken language for translation; auto-detection is unavailable.",
  "下载记录和本地文件已删除": "Download record and local files deleted",
  "下载记录已删除，本地文件已保留": "Record deleted; local files preserved",
  "任务已从原断点重新开始": "Resumed from saved checkpoint",
  "任务已暂停": "Task paused",
  "任务已继续": "Task resumed",
  "全部完成": "All completed",
  "已删除": "Deleted",
  "已打开文件": "File opened",
  "已打开文件所在位置": "Output location opened",
  "当前没有已生成的文件，只会删除这条下载记录。": "No output files exist; only the task record will be deleted.",
  "暂无下载速度": "No download speed yet",
  "未完成": "Incomplete",
  "继续": "Continue",
  "翻译模型正在加载，无法计算百分比": "Loading translation model; percentage unavailable",
  "进行中": "In progress",
  "重新授权并继续": "Reconnect and resume",
  "Bridge 已重启并重新连接。": "Bridge restarted and reconnected.",
  "下载并使用": "Download and use",
  "切换 Tool": "Toggle tool",
  "已下载": "Downloaded",
  "已打开下载目录。": "Opened download directory.",
  "当前": "Current",
  "当前模型": "Current model",
  "推荐": "Recommended",
  "查看模型许可": "View model licence",
  "正在下载…": "Downloading…",
  "正在切换…": "Switching…",
  "没有可用的 Tool 描述。": "No tool description available.",
  "环境异常": "Environment issue",
  "环境正常": "Environment ready",
  "等待检测": "Pending check",
  "设为当前模型": "Set as current model",
  "选择 Whisper 模型": "Select Whisper model"
});

  Object.assign(dictionary, {
  "设置": "Settings",
  "下载、连接、组件与界面偏好": "Downloads, connection, components and preferences",
  "偏好设置": "Preferences",
  "界面与交互": "Appearance & interaction",
  "语言、导航方式与工作台偏好": "Language and workspace navigation",
  "系统与组件": "System & components",
  "连接与运行时": "Connection & runtime",
  "浏览器连接、Bridge 与网络": "Browser, Bridge and networking",
  "下载能力": "Download capabilities",
  "媒体嗅探、下载与处理": "Media detection and downloading",
  "外部组件": "External components",
  "FFmpeg、yt-dlp、识别和翻译": "FFmpeg, yt-dlp, transcription and translation",
  "其他组件": "Other components",
  "其他可选模块": "Additional optional modules",
  "项": "items",
  "就绪": "Ready",
  "未知": "Unknown",
  "等待检测": "Checking",
  "返回上一级": "Back",
  "显示与导航": "Display & navigation",
  "界面语言": "Interface language",
  "工作台与扩展共用此设置": "Shared by workspace and browser extension",
  "扩展打开方式": "Extension open mode",
  "配置统一保存在 Data/config.toml。": "Configuration is stored in Data/config.toml.",
  "保存后更新 TOML 配置": "Save changes to TOML",
  "修改后保存到 Data/config.toml，所有入口共用。": "Changes are stored in Data/config.toml and shared across all views.",
  "查看组件状态与设置": "Component status and settings",
  "暂无可用组件": "No components available",
  "编辑": "Edit",
  "刷新": "Refresh",
  "工作台": "Workspace"
});

  Object.assign(dictionary, {
  "媒体嗅探器": "Media sniffer",
  "Native Host 已连接": "Native Host connected",
  "Bridge 在线": "Bridge online",
  "代理网络通畅": "Proxy connection available",
  "正在检测 HLS、DASH、MP4": "Detecting HLS, DASH, MP4",
  "下载引擎就绪": "Downloader ready",
  "配置已保存": "Configuration saved",
  "编辑 Go 下载引擎": "Edit Go downloader"
});

  Object.assign(dictionary, {"下载选项":"Download options","检测结果":"Detected resources","暂无媒体资源":"No media found","播放网页中的视频后，DownKit 会自动显示可下载资源。":"Play a video in the browser to see downloadable media.","等待打开视频页面":"Waiting for a video page"});

  Object.assign(dictionary, {
    "本地服务": "Local service",
    "浏览器原生连接与 DownKit 下载服务。": "Browser connection and local download service.",
    "浏览器与下载服务连接正常": "Browser and download service connected",
    "本地服务不可用": "Local service unavailable",
    "Native Messaging 已连接": "Native Messaging connected"
  });

  Object.assign(dictionary, {
  "日志": "Logs",
  "运行日志": "Runtime logs",
  "查看 DownKit Bridge 与下载 Worker 的本地运行记录。": "View local Bridge and worker events.",
  "显示日志入口": "Show logs tab",
  "仅隐藏导航入口，不影响日志写入": "Hides navigation only; logging continues",
  "日志入口设置已保存。": "Log tab preference saved.",
  "读取中": "Loading",
  "搜索": "Search",
  "显示级别": "Log level",
  "全部": "All",
  "自动刷新": "Auto-refresh",
  "全选": "Select all",
  "暂无日志": "No logs",
  "事件 / component / error": "Event / component / error",
  "日志已刷新。": "Logs refreshed.",
  "读取失败": "Read failed",
  "已选中当前可见日志；自动刷新已暂停。": "Visible logs selected; auto-refresh paused.",
  "没有可复制的日志。": "No logs to copy.",
  "已复制选中的日志。": "Selected logs copied.",
  "已复制当前可见日志。": "Visible logs copied.",
  "正在清空日志…": "Clearing logs…",
  "日志已清空。": "Logs cleared.",
  "设置": "Settings",
  "日志目录": "Log directory"
});

  let locale = "zh-CN";
  const source = new WeakMap();
  const lastDisplay = new WeakMap();

  function translate(text) {
    if (locale !== "en" || typeof text !== "string") return text;
    const key = text.trim();
    const value = dictionary[key];
    if (!value) {
      const patterns = [
        [/^任务 (.+) 已提交。$/, (_, id) => `Task ${id} submitted.`],
        [/^任务 (.+) 已开始下载。$/, (_, id) => `Task ${id} started.`],
        [/^(.+) 配置已保存。$/, (_, name) => `${dictionary[name] || name} configuration saved.`],
        [/^(.+) 已安装。$/, (_, name) => `${dictionary[name] || name} installed.`],
        [/^(.+) 已开启。$/, (_, name) => `${dictionary[name] || name} enabled.`],
        [/^(.+) 已关闭。$/, (_, name) => `${dictionary[name] || name} disabled.`],
        [/^已复制 (.+) 的完整地址。$/, (_, name) => `Copied the full URL for ${name}.`],
        [/^复制失败：(.*)$/, (_, reason) => `Copy failed: ${reason}`],
        [/^下载速度 (.+)$/, (_, speed) => `Download speed ${speed}`],
        [/^(\d+) 个已完成$/, (_, count) => `${count} completed`],
        [/^已下载 (.+)$/, (_, amount) => `Downloaded ${amount}`],
        [/^(.+) 模型目录$/, (_, name) => `${name} model directory`],
        [/^查看 (.+) 使用条款$/, (_, name) => `View ${name} terms of use`],
        [/^(\d+) 项$/, (_, count) => count + " items"],
        [/^编辑 (.+)$/, (_, tool) => "Edit " + (dictionary[tool] || tool)],
        [/^(.+) 模型 (.+) 已可用并设为当前模型。$/, (_, tool, model) => `${tool} model ${model} is ready and selected.`]
      ];
      for (const [pattern, render] of patterns) {
        if (pattern.test(key)) return text.replace(pattern, render);
      }
      return text;
    }
    const leading = text.match(/^\s*/)?.[0] || "";
    const trailing = text.match(/\s*$/)?.[0] || "";
    return leading + value + trailing;
  }

  function apply(node = document.body) {
    if (!node) return;
    const walk = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
    const nodes = [];
    while (walk.nextNode()) nodes.push(walk.currentNode);
    for (const text of nodes) {
      if (text.parentElement?.closest("script, style, code, pre, textarea")) continue;
      let original = source.get(text);
      const last = lastDisplay.get(text);
      if (original === undefined || (last !== undefined && text.nodeValue !== last)) {
        original = text.nodeValue;
        source.set(text, original);
      }
      const rendered = translate(original);
      lastDisplay.set(text, rendered);
      if (text.nodeValue !== rendered) text.nodeValue = rendered;
    }
    if (node.querySelectorAll) {
      for (const el of node.querySelectorAll("[title],[placeholder],[aria-label]")) {
        for (const attr of ["title", "placeholder", "aria-label"]) {
          const value = el.getAttribute(attr);
          if (value == null) continue;
          const key = "i18n-source-" + attr;
          const previous = el.dataset[key.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase())];
          const original = previous && (value === translate(previous) || value === previous) ? previous : value;
          el.dataset[key.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase())] = original;
          const rendered = translate(original);
          if (value !== rendered) el.setAttribute(attr, rendered);
        }
      }
    }
  }

  let observer;
  function setLocale(next) {
    locale = next === "en" ? "en" : "zh-CN";
    document.documentElement.lang = locale;
    document.getElementById("uiLanguage").value = locale;
    apply();
  }
  async function init() {
    const selector = document.getElementById("uiLanguage");
    try {
      const result = await root.DownKitPopup.send("ui.language.get");
      setLocale(result.locale);
    } catch { setLocale("zh-CN"); }
    selector.addEventListener("change", async () => {
      const selected = selector.value;
      setLocale(selected);
      try { await root.DownKitPopup.send("ui.language.set", { locale: selected }); }
      catch (error) { root.DownKitPopup.setMessage("environmentMessage", error.message, "error"); }
    });
    observer = new MutationObserver(mutations => {
      if (mutations.some(item => item.type === "childList" || item.type === "characterData")) apply();
    });
    observer.observe(document.body, { subtree: true, childList: true, characterData: true });
  }

  root.DownKitI18n = { init, t: translate, apply, get locale() { return locale; } };
})(globalThis);
