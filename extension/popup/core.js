(function (root) {
  "use strict";
  const CONSOLE_ORIGIN = "http://127.0.0.1:17891";
  const isWeb = root.location?.origin === CONSOLE_ORIGIN &&
      root.location?.pathname.startsWith("/console/");
  const pending = new Map();
  let sequence = 0;

  if (isWeb) {
    root.addEventListener("message", event => {
      if (event.source !== root || event.origin !== CONSOLE_ORIGIN ||
          event.data?.channel !== "downkit:console:response:v1") return;
      const request = pending.get(event.data.id);
      if (!request) return;
      clearTimeout(request.timer);
      pending.delete(event.data.id);
      const response = event.data.response;
      if (response?.ok) request.resolve(response);
      else request.reject(new Error(response?.error || "DownKit browser extension is not connected"));
    });
  }

  async function send(action, payload = {}) {
    let response;
    if (isWeb) {
      response = await new Promise((resolve, reject) => {
        const id = `${Date.now()}-${++sequence}`;
        const timer = setTimeout(() => {
          pending.delete(id);
          reject(new Error("DownKit extension is not connected to this Web UI"));
        }, 15000);
        pending.set(id, { resolve, reject, timer });
        root.postMessage({ channel: "downkit:console:request:v1", id, action, payload }, CONSOLE_ORIGIN);
      });
    } else {
      response = await chrome.runtime.sendMessage({ action, ...payload });
      if (!response || !response.ok) throw new Error(response?.error || "DownKit request failed");
    }
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

  root.DownKitPopup = { send, setMessage, formatTime, host: isWeb ? "web" : "extension" };
})(globalThis);
