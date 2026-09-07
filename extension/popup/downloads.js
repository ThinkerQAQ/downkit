(function (root) {
  "use strict";

  const api = root.DownKitPopup;
  let activeTab = null;
  let page = null;
  let items = [];
  let active = false;
  let mediaRefreshTimer = null;

  async function copyText(value) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      try {
        await navigator.clipboard.writeText(value);
        return;
      } catch (_) {
        // Some Chromium policies disable the Clipboard API in extension popups.
        // Fall through to the user-gesture based copy command below.
      }
    }
    const input = document.createElement("textarea");
    input.value = value;
    input.setAttribute("readonly", "");
    input.style.position = "fixed";
    input.style.opacity = "0";
    document.body.appendChild(input);
    input.select();
    const copied = document.execCommand("copy");
    input.remove();
    if (!copied) throw new Error("浏览器未允许复制");
  }

  async function copyResource(resource, button) {
    const originalText = button.textContent;
    try {
      await copyText(resource.url);
      button.textContent = "已复制";
      api.setMessage("downloadMessage", `已复制 ${DownKitMedia.resourceName(resource)} 的完整地址。`, "ok");
    } catch (error) {
      api.setMessage("downloadMessage", `复制失败：${error.message || String(error)}`, "error");
    } finally {
      setTimeout(() => { button.textContent = originalText; }, 1200);
    }
  }

  function renderMedia() {
    const list = document.getElementById("mediaList");
    const template = document.getElementById("mediaTemplate");
    list.replaceChildren();
    document.getElementById("emptyMedia").hidden = items.length > 0;
    for (const resource of items) {
      const node = template.content.firstElementChild.cloneNode(true);
      const name = DownKitMedia.resourceName(resource);
      node.querySelector(".resource-name").textContent = name;
      node.querySelector(".resource-name").title = name;
      node.querySelector(".label").textContent = resource.label;
      node.querySelector(".url").textContent = resource.url;
      node.querySelector(".url").title = resource.url;
      node.querySelector(".copy").addEventListener("click", event => copyResource(resource, event.currentTarget));
      node.querySelector(".download").addEventListener("click", () => download(resource));
      list.appendChild(node);
    }
  }

  async function refreshMedia() {
    [activeTab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (!activeTab || !activeTab.id) throw new Error("找不到当前标签页");
    const response = await api.send("media.list", { tabId: activeTab.id });
    page = response.page;
    items = response.items || [];
    document.getElementById("pageTitle").textContent = page.title || page.url || "当前页面";
    renderMedia();
  }

  function scheduleMediaRefresh() {
    if (!active) return;
    clearTimeout(mediaRefreshTimer);
    mediaRefreshTimer = setTimeout(() => {
      refreshMedia().catch(error => api.setMessage("downloadMessage", error.message || String(error), "error"));
    }, 150);
  }

  function selectedQuality(source) {
    const quality = (source || document).getElementById("quality");
    return quality ? quality.value : "";
  }

  function siteLanguageSelectors(language) {
    const normalized = String(language || "").trim().toLowerCase();
    if (!normalized || normalized === "auto") return ["all", "-live_chat"];
    // Bilibili exposes AI captions as ai-zh, ai-en, etc. yt-dlp lists these
    // as regular subtitles, so --write-auto-subs alone does not make zh.* or
    // en.* match the corresponding AI caption track.
    return [`${normalized}.*`, `ai-${normalized}.*`, normalized];
  }

  function selectedSubtitleRequest(source) {
    const controls = source || document;
    const select = controls.getElementById("subtitleMode");
    const languageSelect = controls.getElementById("asrLanguage");
    const asrLanguage = String(languageSelect && languageSelect.value || "auto").trim().toLowerCase() || "auto";
    let request;
    switch (select ? select.value : "none") {
      case "site-zh":
        request = { mode: "site", languages: ["zh.*", "ai-zh.*", "zh-Hans", "zh-Hant"], includeAutomatic: true, format: "best" };
        break;
      case "site-en":
        request = { mode: "site", languages: ["en.*", "ai-en.*"], includeAutomatic: true, format: "best" };
        break;
      case "site-all":
        request = { mode: "site", languages: ["all", "-live_chat"], includeAutomatic: true, format: "best" };
        break;
      case "site-or-asr":
        request = {
          mode: "site-or-asr",
          languages: siteLanguageSelectors(asrLanguage),
          includeAutomatic: true,
          format: "best",
          asrLanguage
        };
        break;
      case "asr":
        request = { mode: "asr", includeAutomatic: false, format: "srt", asrLanguage };
        break;
      default:
        return { mode: "none" };
    }
    const targetSelect = controls.getElementById("subtitleTargetLanguage");
    const targetLanguage = String(targetSelect && targetSelect.value || "").trim();
    if (targetLanguage) {
      if (["site-or-asr", "asr"].includes(request.mode) && request.asrLanguage === "auto") {
        throw new Error("翻译字幕时，请明确选择视频语音，不能使用自动检测");
      }
      const layoutSelect = controls.getElementById("subtitleLayout");
      request.targetLanguage = targetLanguage;
      request.bilingual = Boolean(layoutSelect && layoutSelect.value === "bilingual");
    }
    return request;
  }

  function syncSubtitleOptionsVisibility(source) {
    const controls = source || document;
    const mode = controls.getElementById("subtitleMode");
    const subtitlesVisible = Boolean(mode && mode.value !== "none");
    const asrVisible = Boolean(mode && ["site-or-asr", "asr"].includes(mode.value));
    for (const id of ["asrLanguageLabel", "asrLanguage", "asrLanguageHint"]) {
      const element = controls.getElementById(id);
      if (element) element.hidden = !asrVisible;
    }
    for (const id of ["subtitleTargetLanguageLabel", "subtitleTargetLanguage", "subtitleTargetLanguageHint"]) {
      const element = controls.getElementById(id);
      if (element) element.hidden = !subtitlesVisible;
    }
    const target = controls.getElementById("subtitleTargetLanguage");
    const layoutVisible = subtitlesVisible && Boolean(target && target.value);
    for (const id of ["subtitleLayoutLabel", "subtitleLayout", "subtitleLayoutHint"]) {
      const element = controls.getElementById(id);
      if (element) element.hidden = !layoutVisible;
    }
    return { subtitlesVisible, asrVisible, layoutVisible };
  }

  async function download(resource) {
    api.setMessage("downloadMessage", "正在检测播放列表…");
    try {
      const quality = selectedQuality();
      const subtitles = selectedSubtitleRequest();
      console.debug("DownKit subtitle request prepared", {
        timestamp: new Date().toISOString(), severity: "DEBUG",
        node: "popup-download", operation: "subtitle.request.build",
        result: subtitles.mode, languages: subtitles.languages || []
      });
      const probe = await api.send("media.playlist.probe", { resource, page, quality });
      const playlist = await root.DownKitPlaylistDialog.choose(probe);
      if (!playlist) {
        api.setMessage("downloadMessage", "已取消下载。");
        return;
      }
      api.setMessage("downloadMessage", "正在读取当前 URL 的 Cookie 并提交任务…");
      const response = await api.send("media.download", {
        resource,
        page,
        quality,
        playlist,
        subtitles
      });
      api.setMessage("downloadMessage", `任务 ${response.taskId} 已提交。`, "ok");
	  await api.activateTab("jobs");
	  api.setMessage("jobsMessage", `任务 ${response.taskId} 已开始下载。`, "ok");
    } catch (error) {
      api.setMessage("downloadMessage", error.message || String(error), "error");
    }
  }

  function init() {
    const subtitleMode = document.getElementById("subtitleMode");
    if (subtitleMode) subtitleMode.addEventListener("change", () => syncSubtitleOptionsVisibility());
    const targetLanguage = document.getElementById("subtitleTargetLanguage");
    if (targetLanguage) targetLanguage.addEventListener("change", () => syncSubtitleOptionsVisibility());
    syncSubtitleOptionsVisibility();
    document.getElementById("clearMedia").addEventListener("click", async () => {
      if (!activeTab) return;
      await api.send("media.clear", { tabId: activeTab.id });
      items = [];
      renderMedia();
    });
    chrome.tabs.onActivated.addListener(scheduleMediaRefresh);
    chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
      if (activeTab && tabId === activeTab.id && (changeInfo.url || changeInfo.status === "complete")) {
        scheduleMediaRefresh();
      }
    });
  }

  async function activate() {
    active = true;
    await refreshMedia().catch(error => api.setMessage("downloadMessage", error.message || String(error), "error"));
  }

  function deactivate() {
    active = false;
    clearTimeout(mediaRefreshTimer);
    mediaRefreshTimer = null;
  }

  root.DownKitDownloads = { init, activate, deactivate };
  if (typeof module !== "undefined" && module.exports) {
    module.exports = { selectedQuality, siteLanguageSelectors, selectedSubtitleRequest, syncSubtitleOptionsVisibility, copyText };
  }
})(globalThis);
