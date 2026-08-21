"use strict";

function activateTab(name) {
  document.querySelectorAll("[data-tab]").forEach(tab => {
    const active = tab.dataset.tab === name;
    tab.classList.toggle("active", active);
    tab.setAttribute("aria-selected", active ? "true" : "false");
    tab.tabIndex = active ? 0 : -1;
  });
  document.querySelectorAll("[data-panel]").forEach(panel => panel.classList.toggle("active", panel.dataset.panel === name));
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
  document.getElementById("extensionVersion").textContent = `v${chrome.runtime.getManifest().version}`;
  DownKitEnvironment.init();
  DownKitJobs.init();
  DownKitDownloads.init();
  initOpenMode();
  document.querySelectorAll("[data-tab]").forEach(tab => tab.addEventListener("click", () => activateTab(tab.dataset.tab)));
  activateTab("sniffer");
});

window.addEventListener("unload", () => {
  DownKitDownloads.deactivate();
  DownKitJobs.deactivate();
});
