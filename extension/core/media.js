(function (root, factory) {
  const api = factory();
  root.DownKitMedia = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  const HLS_TYPES = new Set([
    "application/vnd.apple.mpegurl",
    "application/x-mpegurl",
    "audio/mpegurl",
    "audio/x-mpegurl"
  ]);

  const DEFAULT_SNIFFER_CONFIG = Object.freeze({
    detectHls: true,
    detectDash: true,
    detectMp4: true
  });

  function normalizeSnifferConfig(input) {
    const value = input && typeof input === "object" ? input : {};
    return {
      detectHls: typeof value.detectHls === "boolean" ? value.detectHls : DEFAULT_SNIFFER_CONFIG.detectHls,
      detectDash: typeof value.detectDash === "boolean" ? value.detectDash : DEFAULT_SNIFFER_CONFIG.detectDash,
      detectMp4: typeof value.detectMp4 === "boolean" ? value.detectMp4 : DEFAULT_SNIFFER_CONFIG.detectMp4
    };
  }

  function configAllowsKind(kind, input) {
    const config = normalizeSnifferConfig(input);
    if (kind === "hls") return config.detectHls;
    if (kind === "dash" || kind === "media-track" || kind === "resolved-media-page") return config.detectDash;
    if (kind === "mp4") return config.detectMp4;
    return true;
  }

  function contentType(value) {
    return String(value || "").split(";", 1)[0].trim().toLowerCase();
  }

  function validHTTPURL(value) {
    try {
      const parsed = new URL(String(value || ""));
      return parsed.protocol === "http:" || parsed.protocol === "https:";
    } catch (_) {
      return false;
    }
  }

  function googleVideoPlayback(value) {
    try {
      const parsed = new URL(String(value || ""));
      return (parsed.protocol === "http:" || parsed.protocol === "https:") &&
        (parsed.hostname === "googlevideo.com" || parsed.hostname.endsWith(".googlevideo.com")) &&
        parsed.pathname === "/videoplayback";
    } catch (_) {
      return false;
    }
  }

  function resourceName(input) {
    const resource = input && typeof input === "object" ? input : { url: input };
    try {
      const parsed = new URL(String(resource.url || ""));
      const queryName = parsed.searchParams.get("filename") || parsed.searchParams.get("file") || parsed.searchParams.get("name");
      const encodedName = queryName || parsed.pathname.split("/").filter(Boolean).pop() || "";
      if (encodedName) {
        try {
          return decodeURIComponent(encodedName);
        } catch (_) {
          return encodedName;
        }
      }
    } catch (_) {}

    const fallback = {
      hls: "playlist.m3u8",
      dash: "manifest.mpd",
      mp4: "video.mp4",
      "media-track": "media-track",
      "resolved-media-page": "当前媒体页面"
    };
    return fallback[resource.kind] || "媒体资源";
  }

  function classify(input, config) {
    const url = String(input && input.url || "");
    if (!validHTTPURL(url)) return null;

    const lower = url.toLowerCase();
    const mime = contentType(input && input.contentType);
    let detected = null;
    if (/\.m3u8(?:$|[?#])/i.test(lower) || HLS_TYPES.has(mime)) {
      detected = {
        kind: "hls",
        label: /(?:master|index)\.m3u8(?:$|[?#])/i.test(lower) ? "HLS 清单" : "HLS 媒体清单"
      };
    } else if (/\.mpd(?:$|[?#])/i.test(lower) || mime === "application/dash+xml") {
      detected = { kind: "dash", label: "DASH 清单" };
    } else if (/\.mp4(?:$|[?#])/i.test(lower) || mime === "video/mp4") {
      detected = { kind: "mp4", label: "MP4 视频" };
    } else if (/\.m4s(?:$|[?#])/i.test(lower)) {
      detected = { kind: "media-track", label: "分离媒体轨" };
    } else if (googleVideoPlayback(url)) {
      // YouTube serves its separate audio/video tracks from /videoplayback URLs
      // without a media filename extension. Keep them under the DASH/separate
      // track setting so the UI does not expose a misleading fourth format.
      detected = { kind: "media-track", label: "YouTube 分离媒体轨" };
    }
    return detected && configAllowsKind(detected.kind, config) ? detected : null;
  }

  function hash(value) {
    let result = 2166136261;
    const text = String(value || "");
    for (let i = 0; i < text.length; i += 1) {
      result ^= text.charCodeAt(i);
      result = Math.imul(result, 16777619);
    }
    return (result >>> 0).toString(36);
  }

  const BLOCKED_REQUEST_HEADERS = new Set([
    "accept-encoding", "connection", "content-length", "cookie", "host", "proxy-authorization",
    "proxy-connection", "te", "trailer", "transfer-encoding", "upgrade"
  ]);

  function sanitizeRequestHeaders(input) {
    const result = {};
    if (!input || typeof input !== "object") return result;
    for (const [rawName, rawValue] of Object.entries(input)) {
      const name = String(rawName || "").trim().toLowerCase();
      const value = String(rawValue == null ? "" : rawValue);
      if (!name || BLOCKED_REQUEST_HEADERS.has(name) || /[\r\n]/.test(name + value)) continue;
      result[name] = value;
    }
    return result;
  }

  function makeResource(input, config) {
    const detected = classify(input, config);
    if (!detected) return null;
    const url = String(input.url);
    return {
      id: hash(`${detected.kind}\n${url}`),
      url,
      kind: detected.kind,
      label: detected.label,
      tabId: Number(input.tabId),
      frameId: Number.isInteger(input.frameId) ? input.frameId : 0,
      initiator: String(input.initiator || ""),
      referer: String(input.referer || ""),
      origin: String(input.origin || ""),
      userAgent: String(input.userAgent || ""),
      requestHeaders: sanitizeRequestHeaders(input.requestHeaders),
      statusCode: Number(input.statusCode || 0),
      detectedAt: Number(input.detectedAt || Date.now())
    };
  }

  function displayList(resources, page, config) {
    const items = (Array.isArray(resources) ? resources : []).filter(item => configAllowsKind(item.kind, config));
    const tracks = items.filter(item => item.kind === "media-track");
    const others = items.filter(item => item.kind !== "media-track");
    if (tracks.length >= 2 && page && validHTTPURL(page.url)) {
      const newest = tracks.sort((a, b) => b.detectedAt - a.detectedAt)[0];
      others.unshift({
        ...newest,
        id: "resolved-media-page",
        url: page.url,
        kind: "resolved-media-page",
        label: `媒体页面（已归并 ${tracks.length} 条分离轨）`
      });
    } else {
      others.push(...tracks);
    }
    const rank = { "resolved-media-page": 0, hls: 1, dash: 2, mp4: 3, "media-track": 4 };
    return others.sort((a, b) => {
      const byKind = (rank[a.kind] ?? 9) - (rank[b.kind] ?? 9);
      return byKind || b.detectedAt - a.detectedAt;
    });
  }

  return {
    DEFAULT_SNIFFER_CONFIG,
    normalizeSnifferConfig,
    configAllowsKind,
    classify,
    makeResource,
    displayList,
    resourceName,
    validHTTPURL,
    sanitizeRequestHeaders
  };
});
