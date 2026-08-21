(function () {
  "use strict";

  const sent = new Set();

  function likelyMedia(url) {
    return /\.(?:m3u8|mpd|mp4|m4s)(?:$|[?#])/i.test(String(url || ""));
  }

  function report(url, contentType) {
    const value = String(url || "");
    if (!/^https?:\/\//i.test(value) || !likelyMedia(value) || sent.has(value)) return;
    sent.add(value);
    chrome.runtime.sendMessage({
      action: "media.detected",
      url: value,
      contentType: contentType || "",
      userAgent: navigator.userAgent
    }).catch(() => {});
  }

  function scanElements() {
    for (const element of document.querySelectorAll("video[src], audio[src], source[src]")) {
      report(element.currentSrc || element.src, element.type || "");
    }
  }

  function scanPerformance() {
    for (const entry of performance.getEntriesByType("resource")) report(entry.name, "");
  }

  try {
    const observer = new PerformanceObserver(list => {
      for (const entry of list.getEntries()) report(entry.name, "");
    });
    observer.observe({ type: "resource", buffered: true });
  } catch (_) {}

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => {
      scanElements();
      scanPerformance();
    }, { once: true });
  } else {
    scanElements();
    scanPerformance();
  }

  document.addEventListener("loadedmetadata", scanElements, true);
})();
