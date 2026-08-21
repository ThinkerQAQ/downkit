(function (root, factory) {
  const api = factory();
  root.DownKitLauncher = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function pageOrigin(pageURL) {
    try { return new URL(pageURL).origin; } catch (_) { return ""; }
  }

  function buildTask(resource, page, quality, mediaCookies, pageCookies, playlist, cookieStoreId) {
    if (!resource || !resource.url) throw new Error("缺少媒体 URL");
    const resolveFromPage = resource.kind === "resolved-media-page" || resource.kind === "dash";
    const pageURL = String(page && page.url || "");
    const targetURL = resolveFromPage && /^https?:\/\//i.test(pageURL) ? pageURL : resource.url;
    const mediaHeaders = { ...(resource.requestHeaders || {}) };
    const pageHeaders = {};
    delete mediaHeaders.cookie;
    if (page && page.userAgent) pageHeaders["user-agent"] = String(page.userAgent);
    return {
      url: targetURL,
      title: String(page && page.title || "video"),
      referer: resource.referer || pageURL,
      origin: resource.origin || "",
      userAgent: resource.userAgent || String(page && page.userAgent || ""),
      quality: quality || "",
      playlist: playlist || "ask",
      resolvePage: resolveFromPage,
      mediaHeaders,
      pageHeaders,
      mediaCookies: Array.isArray(mediaCookies) ? mediaCookies : [],
      pageCookies: Array.isArray(pageCookies) ? pageCookies : [],
      cookieStoreId: String(cookieStoreId || ""),
      sourceTabId: Number.isInteger(resource.tabId) ? resource.tabId : -1,
      sourceFrameId: Number.isInteger(resource.frameId) ? resource.frameId : 0,
      pageUrl: pageURL
    };
  }

  return { buildTask };
});
