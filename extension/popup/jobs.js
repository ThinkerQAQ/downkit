(function (root) {
  "use strict";

  const api = root.DownKitPopup;
  let polling = null;
  let active = false;
  const fileExpansionPreferences = new Map();
  const stateLabels = {
    queued: "等待中",
    running: "下载中",
    paused: "已暂停",
    "needs-session": "需重新授权",
    completed: "已完成",
    failed: "失败"
  };
  const phaseLabels = {
    resolving: "解析中",
    downloading: "下载中",
    processing: "处理中"
  };

  function formatBytes(value) {
    if (!Number.isFinite(value) || value <= 0) return "";
    const units = ["B", "KB", "MB", "GB"];
    let size = value;
    let unit = 0;
    while (size >= 1024 && unit < units.length - 1) {
      size /= 1024;
      unit++;
    }
    return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
  }

  function formatSpeed(value) {
    const speed = Number(value) || 0;
    if (speed <= 0) return "0 B/s";
    if (speed < 1024) return `${Math.round(speed)} B/s`;
    if (speed < 1024 * 1024) return `${Math.round(speed / 1024)} KB/s`;
    return `${(speed / (1024 * 1024)).toFixed(1)} MB/s`;
  }

  function totalSpeed(jobs) {
    return (jobs || []).reduce((total, job) => {
      return total + (job.status === "running" ? Math.max(0, Number(job.speedBytesPerSecond) || 0) : 0);
    }, 0);
  }

  function fileName(path) {
    const parts = String(path || "").split(/[\\/]/);
    return parts[parts.length - 1] || "下载文件";
  }

  function jobFiles(job) {
    const files = Array.isArray(job.files) ? job.files.map(file => ({ ...file })) : [];
    const represented = new Set(files.map(file => String(file.outputPath || "").toLocaleLowerCase()).filter(Boolean));
    for (const outputPath of Array.isArray(job.outputPaths) ? job.outputPaths : []) {
      if (represented.has(String(outputPath).toLocaleLowerCase())) continue;
      files.push({ title: fileName(outputPath), status: "completed", outputPath });
    }
    return files;
  }

  function fileSectionExpanded(preferences, jobID, fileCount) {
    return preferences.has(jobID) ? preferences.get(jobID) : fileCount === 1;
  }

  async function chooseDelete(job) {
    const dialog = document.getElementById("deleteJobDialog");
    const outputCount = Array.isArray(job.outputPaths) ? job.outputPaths.length : 0;
    const activeJob = job.status === "running" || job.status === "queued";
    const scope = outputCount > 0
      ? `“仅删除记录”会保留本地文件；“删除记录和文件”会永久删除当前已生成的 ${outputCount} 个文件。`
      : "当前没有已生成的文件，只会删除这条下载记录。";
    dialog.querySelector(".delete-job-description").textContent = `${activeJob ? "当前下载将被停止。\n" : ""}${scope}`;
    dialog.querySelector(".delete-with-files").hidden = outputCount === 0;
    dialog.querySelector(".delete-record-only").textContent = activeJob ? "停止并删除记录" : "仅删除记录";
    dialog.returnValue = "";
    return new Promise(resolve => {
      const closed = () => {
        dialog.removeEventListener("close", closed);
        resolve(dialog.returnValue === "files" ? true : dialog.returnValue === "record" ? false : null);
      };
      dialog.addEventListener("close", closed);
      dialog.showModal();
    });
  }

  async function runAction(button, job, action, payload) {
    button.disabled = true;
    try {
      const result = await api.send(`bridge.job.${action}`, { taskId: job.id, job, ...(payload || {}) });
      const messages = {
        pause: "任务已暂停", resume: "任务已继续", retry: "任务已从原断点重新开始",
        open: "已打开文件", reveal: "已打开文件所在位置",
        delete: payload && payload.deleteFiles ? "下载记录和本地文件已删除" : "下载记录已删除，本地文件已保留"
      };
      api.setMessage("jobsMessage", messages[action], "ok");
      await refresh();
    } catch (error) {
      api.setMessage("jobsMessage", error.message || String(error), "error");
    } finally {
      button.disabled = false;
    }
  }

  function renderJobFiles(node, job) {
    const files = jobFiles(job);
    if (files.length === 0) return;
    const section = node.querySelector(".job-files");
    const toggle = node.querySelector(".job-files-toggle");
    const list = node.querySelector(".job-file-list");
    const completed = files.filter(file => file.outputPath || file.status === "completed").length;
    const expanded = fileSectionExpanded(fileExpansionPreferences, job.id, files.length);
    const listID = `job-files-${job.id}`;
    section.hidden = false;
    toggle.setAttribute("aria-controls", listID);
    toggle.setAttribute("aria-expanded", expanded ? "true" : "false");
    toggle.querySelector(".job-files-summary").textContent = "下载文件";
    toggle.querySelector(".job-files-count").textContent = `${completed}/${files.length}`;
    const filesStatus = toggle.querySelector(".job-files-status");
    filesStatus.textContent = completed === files.length ? "全部完成" : `${completed} 个已完成`;
    section.classList.toggle("all-complete", completed === files.length);
    list.id = listID;
    list.hidden = !expanded;
    toggle.addEventListener("click", () => {
      const willExpand = toggle.getAttribute("aria-expanded") !== "true";
      toggle.setAttribute("aria-expanded", willExpand ? "true" : "false");
      list.hidden = !willExpand;
      fileExpansionPreferences.set(job.id, willExpand);
    });

    const template = document.getElementById("jobFileTemplate");
    files.forEach((file, position) => {
      const fileNode = template.content.firstElementChild.cloneNode(true);
      const index = Number(file.index) > 0 ? Number(file.index) : position + 1;
      const name = file.title || fileName(file.outputPath);
      fileNode.querySelector(".job-file-index").textContent = String(index).padStart(2, "0");
      const nameNode = fileNode.querySelector(".job-file-name");
      nameNode.textContent = name;
      nameNode.title = file.outputPath || name;
      const state = fileNode.querySelector(".job-file-state");
      const complete = Boolean(file.outputPath) || file.status === "completed";
      const stateKey = file.status === "deleted" ? "deleted" : complete ? "completed" : job.status === "failed" ? "failed" : "pending";
      state.textContent = stateKey === "deleted" ? "已删除" : stateKey === "completed" ? "已完成" : stateKey === "failed" ? "未完成" : "等待中";
      state.classList.add(stateKey);
      fileNode.classList.add(stateKey);
      if (file.outputPath) {
        const open = fileNode.querySelector(".job-file-open");
        open.hidden = false;
        open.addEventListener("click", () => runAction(open, job, "open", { outputPath: file.outputPath }));
        const reveal = fileNode.querySelector(".job-file-reveal");
        reveal.hidden = false;
        reveal.addEventListener("click", () => runAction(reveal, job, "reveal", { outputPath: file.outputPath }));
      }
      list.appendChild(fileNode);
    });
  }

  function render(jobs) {
    const list = document.getElementById("jobList");
    const template = document.getElementById("jobTemplate");
    list.replaceChildren();
    document.getElementById("emptyJobs").hidden = jobs.length > 0;
    document.getElementById("overallSpeed").textContent = formatSpeed(totalSpeed(jobs));

    for (const job of jobs) {
      const node = template.content.firstElementChild.cloneNode(true);
      const progress = Math.max(0, Math.min(100, Number(job.progress) || 0));
      node.querySelector(".job-title").textContent = job.title || job.sourceUrl || job.id;
      node.querySelector(".job-title").title = job.sourceUrl || "";
      const state = node.querySelector(".job-state");
      state.textContent = job.status === "running" && phaseLabels[job.phase]
        ? phaseLabels[job.phase]
        : stateLabels[job.status] || job.status;
      state.classList.add(job.status || "queued");
      node.querySelector(".job-meta").textContent = `${job.id} · ${api.formatTime(job.updatedAt || job.createdAt)}`;

      const percent = node.querySelector(".job-percent");
      percent.textContent = `${progress}%`;
      const bar = node.querySelector(".job-progress-value");
      bar.style.width = `${progress}%`;
      bar.parentElement.setAttribute("aria-valuenow", String(progress));
      const speed = job.status === "running" ? Math.max(0, Number(job.speedBytesPerSecond) || 0) : 0;
      const speedElement = node.querySelector(".job-speed");
      speedElement.textContent = speed > 0 ? formatSpeed(speed) : "—";
      speedElement.setAttribute("aria-label", speed > 0 ? `下载速度 ${formatSpeed(speed)}` : "暂无下载速度");
	  const downloadedText = formatBytes(Number(job.downloadedBytes) || 0);
	  const byteText = job.totalBytes > 0
		? ` · ${downloadedText} / ${formatBytes(job.totalBytes)}`
		: (downloadedText ? ` · 已下载 ${downloadedText}` : "");
      const phasePrefix = job.phase === "downloading" || job.phase === "processing" ? "解析 100% · " : "";
      node.querySelector(".job-detail").textContent = `${phasePrefix}${job.detail || stateLabels[job.status] || ""}${byteText}`;

      if (job.error) {
        const error = node.querySelector(".job-error");
        error.hidden = false;
        error.textContent = job.error;
      }

      const primaryAction = node.querySelector(".job-primary-action");
      if (job.status === "running" || job.status === "queued") {
        primaryAction.hidden = false;
        primaryAction.textContent = "暂停";
        primaryAction.addEventListener("click", () => runAction(primaryAction, job, "pause"));
      } else if (job.status === "paused" || job.status === "needs-session") {
        primaryAction.hidden = false;
        primaryAction.textContent = job.status === "needs-session" ? "重新授权并继续" : "继续";
        primaryAction.addEventListener("click", () => runAction(primaryAction, job, "resume"));
      } else if (job.status === "failed") {
        primaryAction.hidden = false;
        primaryAction.textContent = "重试";
        primaryAction.addEventListener("click", () => runAction(primaryAction, job, "retry"));
      }

      renderJobFiles(node, job);
      const remove = node.querySelector(".job-delete");
      remove.addEventListener("click", async () => {
        const deleteFiles = await chooseDelete(job);
        if (deleteFiles === null) return;
        await runAction(remove, job, "delete", { deleteFiles });
      });
      list.appendChild(node);
    }
  }

  async function refresh() {
    try {
      const response = await api.send("bridge.jobs.list");
      render(response.jobs || []);
    } catch (error) {
      api.setMessage("jobsMessage", error.message || String(error), "error");
    }
  }

  function init() {
    document.getElementById("refreshJobs").addEventListener("click", refresh);
    document.getElementById("clearJobs").addEventListener("click", async () => {
      try {
        await api.send("bridge.jobs.clear");
        await refresh();
      } catch (error) {
        api.setMessage("jobsMessage", error.message || String(error), "error");
      }
    });
  }

  async function activate() {
    active = true;
    await refresh();
    if (active && !polling) polling = setInterval(refresh, 1000);
  }

  function deactivate() {
    active = false;
    if (polling) clearInterval(polling);
    polling = null;
  }

  root.DownKitJobs = { init, activate, deactivate, refresh, render, formatBytes, formatSpeed, totalSpeed };
  if (typeof module !== "undefined" && module.exports) {
    module.exports = { formatBytes, formatSpeed, totalSpeed, fileName, jobFiles, fileSectionExpanded };
  }
})(globalThis);
