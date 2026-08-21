(function (root, factory) {
  const api = factory();
  root.DownKitCookies = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function structured(cookies) {
    const seen = new Set();
    return (cookies || []).filter(cookie => {
      const key = [cookie.name, cookie.domain, cookie.path, JSON.stringify(cookie.partitionKey || null)].join("\n");
      if (!cookie.name || seen.has(key)) return false;
      seen.add(key);
      return true;
    }).map(cookie => ({
      name: String(cookie.name || ""),
      value: String(cookie.value || ""),
      domain: String(cookie.domain || ""),
      hostOnly: Boolean(cookie.hostOnly),
      path: String(cookie.path || "/"),
      secure: Boolean(cookie.secure),
      httpOnly: Boolean(cookie.httpOnly),
      sameSite: String(cookie.sameSite || "unspecified"),
      session: Boolean(cookie.session),
      ...(Number.isFinite(cookie.expirationDate) ? { expirationDate: Number(cookie.expirationDate) } : {}),
      ...(cookie.partitionKey ? { partitionKey: cookie.partitionKey } : {})
    }));
  }

  async function storeIDForTab(api, tabId) {
    if (!Number.isInteger(tabId) || tabId < 0) return "";
    const stores = await api.getAllCookieStores();
    const store = stores.find(item => Array.isArray(item.tabIds) && item.tabIds.includes(tabId));
    return String(store && store.id || "");
  }

  async function forURL(api, rawURL, tabId, frameId, preferredStoreId) {
    let parsed;
    try { parsed = new URL(String(rawURL || "")); } catch (_) { return ""; }
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return "";

    const storeId = String(preferredStoreId || await storeIDForTab(api, tabId));
    const filter = { url: parsed.href, ...(storeId ? { storeId } : {}) };
    const cookies = await api.getAll(filter);

    if (Number.isInteger(tabId) && tabId >= 0 && api.getPartitionKey) {
      try {
        const partitionKey = await api.getPartitionKey({
          tabId,
          frameId: Number.isInteger(frameId) && frameId >= 0 ? frameId : 0
        });
        if (partitionKey && partitionKey.topLevelSite) {
          cookies.push(...await api.getAll({ ...filter, partitionKey }));
        }
      } catch (_) {
        // Older Chromium builds or a frame that has already disappeared: the
        // unpartitioned cookies above remain a safe fallback.
      }
    }
    return structured(cookies);
  }

  return { structured, storeIDForTab, forURL };
});
