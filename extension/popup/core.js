(function (root) {
  "use strict";

  async function send(action, payload) {
    const response = await chrome.runtime.sendMessage({ action, ...(payload || {}) });
    if (!response || !response.ok) throw new Error(response && response.error || "操作失败");
    return response;
  }

  function setMessage(id, text, type) {
    const element = document.getElementById(id);
    if (!element) return;
    element.textContent = text || "";
    element.className = `message${type ? ` ${type}` : ""}`;
  }

  function formatTime(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "";
    return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  }

  root.DownKitPopup = { send, setMessage, formatTime };
})(globalThis);
