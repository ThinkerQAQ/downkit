"use strict";

(function initializeSurface(root) {
  function detectSurface(chromeAPI, currentWindow) {
    const extensionAPI = chromeAPI && chromeAPI.extension;
    if (!extensionAPI || typeof extensionAPI.getViews !== "function") return "popup";
    try {
      const popupViews = extensionAPI.getViews({ type: "popup" });
      return Array.isArray(popupViews) && popupViews.includes(currentWindow) ? "popup" : "side-panel";
    } catch {
      return "popup";
    }
  }

  const surface = root.location?.origin === "http://127.0.0.1:17891" &&
    root.location?.pathname.startsWith("/console/") ? "web" : detectSurface(root.chrome, root);
  if (root.document && root.document.documentElement) {
    root.document.documentElement.dataset.surface = surface;
    root.console.debug("DownKit UI surface detected", {
      timestamp: new Date().toISOString(),
      operation: "ui.surface.detect",
      result: surface,
      viewportWidth: root.innerWidth
    });
  }

  if (typeof module !== "undefined" && module.exports) module.exports = { detectSurface };
})(typeof globalThis !== "undefined" ? globalThis : this);
