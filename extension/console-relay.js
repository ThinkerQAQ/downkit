"use strict";
// Fixed loopback-only relay. The web page never receives the native Bridge token.
(() => {
  const ORIGIN = "http://127.0.0.1:17891";
  if (location.origin !== ORIGIN || !location.pathname.startsWith("/console/") || window.top !== window) return;
  const allowed = new Set([
    "ui.open-mode.get", "ui.open-mode.set", "ui.active-tab",
    "ui.language.get", "ui.language.set",
    "media.list", "media.clear", "media.download", "media.playlist.probe",
    "bridge.environment", "bridge.tools", "bridge.tool.action", "bridge.config.save",
    "tool.config.save", "bridge.jobs.list", "bridge.jobs.clear",
    "bridge.job.retry", "bridge.job.resume", "bridge.job.pause", "bridge.job.open",
    "bridge.job.reveal", "bridge.job.delete", "bridge.output.open"
  ]);
  window.addEventListener("message", event => {
    if (event.source !== window || event.origin !== ORIGIN) return;
    const request = event.data;
    if (request?.channel !== "downkit:console:request:v1" ||
        typeof request.id !== "string" || request.id.length > 96 ||
        !allowed.has(request.action) ||
        !request.payload || typeof request.payload !== "object" || Array.isArray(request.payload)) return;
    chrome.runtime.sendMessage({ ...request.payload, action: request.action }, response => {
      const error = chrome.runtime.lastError;
      window.postMessage({
        channel: "downkit:console:response:v1",
        id: request.id,
        response: error ? { ok: false, error: error.message } : (response || { ok: false, error: "Bridge unavailable" })
      }, ORIGIN);
    });
  });
})();
