"use strict";

function activateTab(name) {
  document.querySelectorAll("[data-tab]").forEach(tab => {
    const active = tab.dataset.tab === name;
    tab.classList.toggle("active", active);
    tab.setAttribute("aria-selected", active ? "true" : "false");
    tab.tabIndex = active ? 0 : -1;
  });
  document.querySelectorAll("[data-panel]").forEach(panel => {
    const active = panel.dataset.panel === name;
    panel.classList.toggle("active", active);
    panel.hidden = !active;
  });
  DownKitDownloads.deactivate();
  DownKitJobs.deactivate();
	if (name === "environment") return DownKitEnvironment.activate();
	if (name === "sniffer") return DownKitDownloads.activate();
	if (name === "jobs") return DownKitJobs.activate();
}

DownKitPopup.activateTab = activateTab;

async function initOpenMode() {
  const select = document.getElementById("openMode");
  const hint = document.getElementById("openModeHint");
  try {
    const result = await DownKitPopup.send("ui.open-mode.get");
    select.value = result.mode;
    if (!result.supportsSidePanel) {
      select.querySelector('[value="sidePanel"]').disabled = true;
      hint.textContent = "当前浏览器不支持侧边栏";
    }
  } catch (error) {
    select.disabled = true;
    hint.textContent = error.message || String(error);
  }
  select.addEventListener("change", async () => {
    select.disabled = true;
    hint.textContent = "正在保存…";
    try {
      const result = await DownKitPopup.send("ui.open-mode.set", { mode: select.value });
      select.value = result.mode;
      hint.textContent = "已保存，下次点击扩展图标时生效";
    } catch (error) {
      hint.textContent = error.message || String(error);
    } finally {
      select.disabled = false;
    }
  });
}

document.addEventListener("DOMContentLoaded", () => {
  document.getElementById("extensionVersion").textContent =
    DownKitPopup.host === "web" ? "Web UI" : `v${chrome.runtime.getManifest().version}`;
  const open = document.getElementById("openWorkspace");
  if (DownKitPopup.host === "web") open.hidden = true;
  open.addEventListener("click", async () => {
    try {
      await DownKitPopup.send("bridge.environment");
      await chrome.tabs.create({ url: "http://127.0.0.1:17891/console/" });
    } catch (error) {
      DownKitPopup.setMessage("environmentMessage", error.message || String(error), "error");
    }
  });
  document.getElementById("refreshWorkspace").addEventListener("click", () => activateTab(
    document.querySelector(".tab.active")?.dataset.tab || "sniffer"));
  DownKitI18n.init();
  DownKitEnvironment.init();
  DownKitJobs.init();
  DownKitDownloads.init();
  initOpenMode();
  const tabs = [...document.querySelectorAll("[data-tab]")];
  for (const [index, tab] of tabs.entries()) {
    tab.addEventListener("click", () => activateTab(tab.dataset.tab));
    tab.addEventListener("keydown", event => {
      if (!["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      const next = event.key === "Home" ? 0 :
        event.key === "End" ? tabs.length - 1 :
          (index + (["ArrowRight", "ArrowDown"].includes(event.key) ? 1 : -1) + tabs.length) % tabs.length;
      tabs[next].focus();
      activateTab(tabs[next].dataset.tab);
    });
  }
  if (DownKitPopup.host === "web") activateTab("jobs");
  else activateTab("sniffer");
});

window.addEventListener("unload", () => {
  DownKitDownloads.deactivate();
  DownKitJobs.deactivate();
});
