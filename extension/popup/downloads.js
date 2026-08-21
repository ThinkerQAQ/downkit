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

  async function download(resource) {
    api.setMessage("downloadMessage", "正在检测播放列表…");
    try {
      const quality = selectedQuality();
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
        playlist
      });
      api.setMessage("downloadMessage", `任务 ${response.taskId} 已提交。`, "ok");
	  await api.activateTab("jobs");
	  api.setMessage("jobsMessage", `任务 ${response.taskId} 已开始下载。`, "ok");
    } catch (error) {
      api.setMessage("downloadMessage", error.message || String(error), "error");
    }
  }

  function init() {
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
    module.exports = { selectedQuality, copyText };
  }
})(globalThis);
