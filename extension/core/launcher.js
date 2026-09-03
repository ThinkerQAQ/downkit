(function (root, factory) {
  const api = factory();
  root.DownKitLauncher = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function pageOrigin(pageURL) {
    try { return new URL(pageURL).origin; } catch (_) { return ""; }
  }

  function normalizeSubtitleRequest(value) {
    const request = value && typeof value === "object" ? value : {};
    const modes = new Set(["site", "asr", "site-or-asr"]);
    if (!modes.has(request.mode)) return { mode: "none" };
    const languages = Array.isArray(request.languages)
      ? [...new Set(request.languages.map(item => String(item || "").trim()).filter(Boolean))].slice(0, 20)
      : [];
    return {
      mode: request.mode,
      ...(["site", "site-or-asr"].includes(request.mode) ? { languages: languages.length ? languages : ["all", "-live_chat"] } : {}),
      includeAutomatic: Boolean(request.includeAutomatic),
      format: ["best", "srt", "vtt", "ass"].includes(request.format) ? request.format : "best",
      ...(["asr", "site-or-asr"].includes(request.mode) ? { asrLanguage: String(request.asrLanguage || "auto").trim().toLowerCase() || "auto" } : {})
    };
  }

  function buildTask(resource, page, quality, mediaCookies, pageCookies, playlist, cookieStoreId, subtitles) {
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
      pageUrl: pageURL,
      subtitles: normalizeSubtitleRequest(subtitles)
    };
  }

  return { buildTask, normalizeSubtitleRequest };
});
