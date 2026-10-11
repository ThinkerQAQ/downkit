"use strict";

const DownKitLogs = (() => {
  const state = {
    initialized: false,
    active: false,
    entries: [],
    path: "",
    configuredLevel: "",
    pollTimer: null,
    pointerSelecting: false,
  };

  let status, pathValue, output, queryInput, levelFilter, autoRefresh, selectAllButton, copyButton, refreshButton, clearButton, message;

  function setStatus(element, type, label, detail) {
    element.className = "log-status " + type;
    element.textContent = label;
    element.title = detail || "";
  }

  function formatAttribute(value) {
    if (typeof value === "string") return value;
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }

  function rawEntryParts(entry) {
    const raw = String(entry?.raw || "");
    const match = raw.match(/^(\[[^\]]+\])\s+(DEBUG|INFO|WARN|ERROR)\s*(.*)$/u);
    if (!match) return { time: "", level: "", body: raw };
    return { time: match[1], level: match[2], body: match[3] };
  }

  function entryLevel(entry) {
    const explicit = String(entry?.level || "").trim().toUpperCase();
    return explicit || rawEntryParts(entry).level;
  }

  function formatEntry(entry) {
    if (entry?.raw) return String(entry.raw);
    const time = entry?.time ? DownKitPopup.formatTime(entry.time) : "--:--:--";
    const level = String(entry?.level || "INFO").toUpperCase().padEnd(5, " ");
    const text = String(entry?.message || "");
    const attributes = Object.entries(entry?.attributes || {})
      .map(([key, value]) => `${key}=${formatAttribute(value)}`)
      .join(" ");
    return [`[${time}]`, level, text, attributes].filter(Boolean).join(" ");
  }

  function filteredEntries() {
    const selected = String(levelFilter?.value || "").toUpperCase();
    const query = String(queryInput?.value || "").trim().toLowerCase();
    return state.entries.filter((entry) => {
      if (selected && entryLevel(entry) !== selected) return false;
      if (query && !formatEntry(entry).toLowerCase().includes(query)) return false;
      return true;
    });
  }

  function appendHighlightedBody(container, value) {
    const text = String(value || "");
    const pattern = /(https?:\/\/[^\s]+)|(\b[A-Za-z][A-Za-z0-9_.-]*=)/gu;
    let offset = 0;
    for (const match of text.matchAll(pattern)) {
      const index = match.index ?? 0;
      if (index > offset) container.append(document.createTextNode(text.slice(offset, index)));
      const token = document.createElement("span");
      token.className = match[1] ? "log-url" : "log-key";
      token.textContent = match[0];
      container.append(token);
      offset = index + match[0].length;
    }
    if (offset < text.length) container.append(document.createTextNode(text.slice(offset)));
  }

  function renderEntry(entry) {
    const line = document.createElement("span");
    const raw = entry?.raw ? rawEntryParts(entry) : null;
    const level = (raw?.level || entryLevel(entry) || "INFO").toUpperCase();
    line.className = `log-line log-line-${level.toLowerCase()}`;
    line.dataset.level = level;

    const time = document.createElement("span");
    time.className = "log-time";
    time.textContent = raw?.time || `[${entry?.time ? DownKitPopup.formatTime(entry.time) : "--:--:--"}]`;

    const levelToken = document.createElement("span");
    levelToken.className = "log-level";
    levelToken.textContent = level.padEnd(5, " ");

    const body = document.createElement("span");
    body.className = "log-body";
    if (raw) {
      appendHighlightedBody(body, raw.body);
    } else {
      const text = String(entry?.message || "");
      const attributes = Object.entries(entry?.attributes || {})
        .map(([key, value]) => `${key}=${formatAttribute(value)}`)
        .join(" ");
      appendHighlightedBody(body, [text, attributes].filter(Boolean).join(" "));
    }

    line.append(time, document.createTextNode(" "), levelToken, document.createTextNode(" "), body);
    return line;
  }

  function selectedLogText() {
    const selection = globalThis.getSelection?.();
    if (!selection || selection.isCollapsed || !selection.anchorNode || !selection.focusNode) return "";
    if (!output.contains(selection.anchorNode) || !output.contains(selection.focusNode)) return "";
    return selection.toString();
  }

  function selectionLocksViewer() {
    return state.pointerSelecting || selectedLogText() !== "";
  }

  function renderOutput(entries) {
    const wasNearBottom = output.scrollHeight - output.scrollTop - output.clientHeight < 36;
    const fragment = document.createDocumentFragment();

    if (!entries.length) {
      fragment.append(document.createTextNode("暂无日志"));
    } else {
      entries.forEach((entry, index) => {
        fragment.append(renderEntry(entry));
        if (index < entries.length - 1) fragment.append(document.createTextNode("\n"));
      });
    }

    output.replaceChildren(fragment);
    if (wasNearBottom || output.textContent === "暂无日志") {
      output.scrollTop = output.scrollHeight;
    }
  }

  function render({ preserveSelection = false } = {}) {
    const entries = filteredEntries();
    if (!(preserveSelection && selectionLocksViewer())) {
      renderOutput(entries);
    }
    pathValue.textContent = state.path || "-";
    setStatus(
      status,
      "ok",
      `${String(state.configuredLevel || "info").toUpperCase()} · ${entries.length}${entries.length !== state.entries.length ? ` / ${state.entries.length}` : ""} 条`,
      state.path || "",
    );
  }

  async function refresh({ quiet = false } = {}) {
    if (!state.active && quiet) return;
    if (state.refreshing) return;
    state.refreshing = true;
    if (!quiet) refreshButton.disabled = true;
    try {
      const response = await DownKitPopup.send("bridge.logs.list", { limit: 1000 });
      state.entries = Array.isArray(response.entries) ? response.entries : [];
      state.path = String(response.path || "");
      state.configuredLevel = String(response.level || "info");
      render({ preserveSelection: quiet });
      if (!quiet) DownKitPopup.setMessage("logsMessage", "日志已刷新。", "ok");
    } catch (error) {
      setStatus(status, "error", "读取失败", String(error?.message || error));
      if (!quiet) DownKitPopup.setMessage("logsMessage", String(error?.message || error), "error");
    } finally {
      state.refreshing = false;
      refreshButton.disabled = false;
    }
  }

  function stopPolling() {
    if (state.pollTimer) {
      clearInterval(state.pollTimer);
      state.pollTimer = null;
    }
  }

  function syncPolling() {
    stopPolling();
    if (!state.active || !autoRefresh.checked) return;
    state.pollTimer = setInterval(() => {
      refresh({ quiet: true });
    }, 1500);
  }

  function selectAllLogs() {
    if (!output || output.textContent === "暂无日志") return;
    if (autoRefresh?.checked) {
      autoRefresh.checked = false;
      syncPolling();
    }
    const selection = globalThis.getSelection?.();
    if (!selection) return;
    const range = document.createRange();
    range.selectNodeContents(output);
    selection.removeAllRanges();
    selection.addRange(range);
    DownKitPopup.setMessage("logsMessage", "已选中当前可见日志；自动刷新已暂停。", "ok");
  }

  async function writeClipboard(text) {
    if (globalThis.navigator?.clipboard?.writeText) {
      await globalThis.navigator.clipboard.writeText(text);
      return;
    }
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.append(textarea);
    textarea.select();
    const copied = document.execCommand("copy");
    textarea.remove();
    if (!copied) throw new Error("浏览器拒绝复制日志");
  }

  async function copyLogs() {
    const selected = selectedLogText();
    const text = selected || (output?.textContent === "暂无日志" ? "" : output?.textContent || "");
    if (!text) {
      DownKitPopup.setMessage("logsMessage", "没有可复制的日志。", "error");
      return;
    }
    copyButton.disabled = true;
    try {
      await writeClipboard(text);
      DownKitPopup.setMessage("logsMessage", selected ? "已复制选中的日志。" : "已复制当前可见日志。", "ok");
    } catch (error) {
      DownKitPopup.setMessage("logsMessage", String(error?.message || error), "error");
    } finally {
      copyButton.disabled = false;
    }
  }

  async function clearLogs() {
    clearButton.disabled = true;
    DownKitPopup.setMessage("logsMessage", "正在清空日志…");
    try {
      await DownKitPopup.send("bridge.logs.clear");
      await refresh({ quiet: true });
      DownKitPopup.setMessage("logsMessage", "日志已清空。", "ok");
    } catch (error) {
      DownKitPopup.setMessage("logsMessage", String(error?.message || error), "error");
    } finally {
      clearButton.disabled = false;
    }
  }

  function init() {
    if (state.initialized) return;
    status = document.getElementById("logsStatus");
    pathValue = document.getElementById("logPath");
    output = document.getElementById("logOutput");
    queryInput = document.getElementById("logQuery");
    levelFilter = document.getElementById("logLevelFilter");
    autoRefresh = document.getElementById("logAutoRefresh");
    selectAllButton = document.getElementById("selectAllLogs");
    copyButton = document.getElementById("copyLogs");
    refreshButton = document.getElementById("refreshLogs");
    clearButton = document.getElementById("clearLogs");
    message = document.getElementById("logsMessage");

    refreshButton.addEventListener("click", () => refresh());
    clearButton.addEventListener("click", clearLogs);
    selectAllButton.addEventListener("click", selectAllLogs);
    copyButton.addEventListener("mousedown", (event) => event.preventDefault());
    copyButton.addEventListener("click", copyLogs);
    queryInput.addEventListener("input", () => render());
    levelFilter.addEventListener("change", () => render());
    autoRefresh.addEventListener("change", syncPolling);
    output.addEventListener("pointerdown", () => {
      state.pointerSelecting = true;
    });
    globalThis.addEventListener("pointerup", () => {
      state.pointerSelecting = false;
    });
    globalThis.addEventListener("pointercancel", () => {
      state.pointerSelecting = false;
    });
    state.initialized = true;
  }

  function activate() {
    state.active = true;
    refresh({ quiet: true });
    syncPolling();
  }

  function deactivate() {
    state.active = false;
    stopPolling();
    state.pointerSelecting = false;
  }

  return { init, activate, deactivate, refresh };
})();
