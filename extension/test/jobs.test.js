"use strict";

const assert = require("node:assert/strict");
global.DownKitPopup = {};
const jobs = require("../popup/jobs.js");

assert.equal(jobs.formatBytes(1024), "1.0 KB");
assert.equal(jobs.formatBytes(5 * 1024 * 1024), "5.0 MB");
assert.equal(jobs.formatBytes(0), "");
assert.equal(jobs.formatSpeed(5 * 1024 * 1024), "5.0 MB/s");
assert.equal(jobs.formatSpeed(512 * 1024), "512 KB/s");
assert.equal(jobs.formatSpeed(0), "0 B/s");
assert.equal(jobs.totalSpeed([
  { status: "running", speedBytesPerSecond: 5 * 1024 * 1024 },
  { status: "running", speedBytesPerSecond: 2 * 1024 * 1024 },
  { status: "paused", speedBytesPerSecond: 9 * 1024 * 1024 }
]), 7 * 1024 * 1024);
assert.equal(jobs.fileName("C:\\Downloads\\Course\\001 - lesson.mp4"), "001 - lesson.mp4");
assert.equal(jobs.fileName("/downloads/lesson.mp4"), "lesson.mp4");
assert.deepEqual(jobs.jobFiles({
  files: [{ index: 1, title: "第一集", status: "pending" }],
  outputPaths: ["C:\\Downloads\\second.mp4"]
}), [
  { index: 1, title: "第一集", status: "pending" },
  { title: "second.mp4", status: "completed", outputPath: "C:\\Downloads\\second.mp4" }
]);
const expansionPreferences = new Map();
assert.equal(jobs.fileSectionExpanded(expansionPreferences, "single", 1), true);
assert.equal(jobs.fileSectionExpanded(expansionPreferences, "playlist", 2), false);
expansionPreferences.set("single", false);
assert.equal(jobs.fileSectionExpanded(expansionPreferences, "single", 1), false);
expansionPreferences.set("playlist", true);
assert.equal(jobs.fileSectionExpanded(expansionPreferences, "playlist", 2), true);

const source = require("node:fs").readFileSync(require("node:path").join(__dirname, "../popup/jobs.js"), "utf8");
assert.match(source, /resolving:\s*"解析中"/);
assert.match(source, /解析 100%/);
assert.match(source, /已下载/);
assert.match(source, /仅删除记录/);
assert.match(source, /删除记录和文件/);
assert.match(source, /filesStatus\.textContent = completed === files\.length \? "全部完成"/);
assert.match(source, /fileNode\.classList\.add\(stateKey\)/);
assert.match(source, /fileExpansionPreferences\.set\(job\.id, willExpand\)/);

console.log("extension jobs tests passed");
