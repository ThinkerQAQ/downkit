(function (root, factory) {
  const api = factory();
  root.DownKitStore = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  const PREFIX = "media.tab.";
  const MAX_ITEMS = 120;
  const MAX_AGE_MS = 6 * 60 * 60 * 1000;

  function key(tabId) { return `${PREFIX}${tabId}`; }

  async function get(tabId) {
    const storageKey = key(tabId);
    const result = await chrome.storage.session.get(storageKey);
    const now = Date.now();
    return (Array.isArray(result[storageKey]) ? result[storageKey] : [])
      .filter(item => now - Number(item.detectedAt || 0) < MAX_AGE_MS);
  }

  async function put(resource) {
    const items = await get(resource.tabId);
    const old = items.find(item => item.id === resource.id);
    const merged = old ? { ...old, ...resource, detectedAt: Date.now() } : resource;
    const next = [merged, ...items.filter(item => item.id !== resource.id)].slice(0, MAX_ITEMS);
    await chrome.storage.session.set({ [key(resource.tabId)]: next });
    return next;
  }

  async function clear(tabId) {
    await chrome.storage.session.remove(key(tabId));
  }

  return { get, put, clear };
});
