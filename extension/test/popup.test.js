"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const html = fs.readFileSync(path.join(__dirname, "../popup/popup.html"), "utf8");
const popupSource = fs.readFileSync(path.join(__dirname, "../popup/popup.js"), "utf8");
const tabs = [...html.matchAll(/data-tab="([^"]+)"/g)].map(match => match[1]);
const panels = [...html.matchAll(/data-panel="([^"]+)"/g)].map(match => match[1]);

assert.deepEqual(tabs, ["sniffer", "jobs", "environment"]);
assert.deepEqual(panels, tabs);
assert.match(popupSource, /activateTab\("sniffer"\);/);
assert.match(html, />资源嗅探<\/button>/);
assert.match(html, />下载任务<\/button>/);
assert.doesNotMatch(html, /id="restartBridge"/);
assert.doesNotMatch(html, /id="saveConfig"/);
assert.doesNotMatch(html, /id="openOutput"/);
assert.doesNotMatch(html, /嗅探与任务/);
assert.match(html, /id="openMode"/);
assert.match(html, /id="playlistDialog"/);
assert.match(html, /id="deleteJobDialog"/);
assert.match(html, /id="jobFileTemplate"/);
assert.match(html, /value="single">仅下载当前视频/);
assert.match(html, /value="all">下载全部视频/);
assert.match(html, /value="popup">弹出窗口（默认）/);
assert.match(html, /value="sidePanel">浏览器侧边栏/);
assert.match(html, /class="secondary copy"/);
assert.match(html, /class="resource-name"/);
const mediaScriptIndex = html.indexOf('<script src="../core/media.js"></script>');
const downloadsScriptIndex = html.indexOf('<script src="downloads.js"></script>');
assert.ok(mediaScriptIndex >= 0, "popup must load the shared media helpers");
assert.ok(mediaScriptIndex < downloadsScriptIndex, "media helpers must load before downloads.js");
const environmentSource = fs.readFileSync(path.join(__dirname, "../popup/environment.js"), "utf8");
const popupStyles = fs.readFileSync(path.join(__dirname, "../popup/popup.css"), "utf8");
const surfaceScriptIndex = html.indexOf('<script src="surface.js"></script>');
const stylesheetIndex = html.indexOf('<link rel="stylesheet" href="popup.css">');
assert.ok(surfaceScriptIndex >= 0, "popup must detect its browser surface");
assert.ok(surfaceScriptIndex < stylesheetIndex, "surface detection must run before popup styles load");
assert.match(environmentSource, /defaultExpanded/);
assert.doesNotMatch(environmentSource, /function editIcon/);
assert.match(environmentSource, /className = "tool-card-body-header"/);
assert.match(environmentSource, /className = "secondary tool-edit"/);
assert.match(environmentSource, /controls\.append\(cancelButton, saveButton\)/);
assert.match(environmentSource, /editorValue\(tool, field\)/);
assert.doesNotMatch(environmentSource, /editing \? "×"/);

assert.match(popupStyles, /body \{ min-width: 280px; width: 100%;/);
assert.match(popupStyles, /html\[data-surface="popup"\].*min-width: 560px; width: 560px;/);
assert.doesNotMatch(popupStyles, /@media \(max-width: 559px\)/);
assert.match(popupStyles, /\.tool-card-body-header \{/);
assert.match(popupStyles, /\.tool-edit-controls \{/);
assert.match(html, /class="job-files-status"/);
assert.match(html, /class="job-output-action job-file-open"/);
assert.match(popupStyles, /\.job-files\.all-complete/);
assert.match(popupStyles, /\.job-file-state\.completed::before/);

console.log("extension popup tests passed");
