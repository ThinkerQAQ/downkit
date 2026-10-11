"use strict";

const assert = require("assert");

global.DownKitPopup = {};
const environment = require("../popup/environment.js");

assert.equal(environment.cardState({ status: "ready", ok: true }), "ok");
assert.equal(environment.cardState({ status: "disabled", ok: true }), "disabled");
assert.equal(environment.cardState({ status: "missing", ok: false }), "error");
assert.equal(environment.cardState({ status: "incompatible", ok: false }), "error");
assert.equal(environment.cardState({ status: "unsupported", ok: false }), "checking");
assert.equal(environment.cardState(null), "checking");
assert.equal(environment.formatModelSize(147951465), "141 MiB");
assert.equal(environment.modelActionLabel({ active: true }), "当前模型");
assert.equal(environment.modelActionLabel({ installed: true }), "设为当前模型");
assert.equal(environment.modelActionLabel({}), "下载并使用");
assert.match(environment.modelOptionLabel({ id: "base", language: "多语言（含中文）", variant: "标准版", sizeBytes: 147951465, recommended: true }), /base.*141 MiB.*推荐/);
assert.equal(environment.normalizedKind({ kind: "infra" }), "runtime");
assert.equal(environment.normalizedKind({ kind: "core" }), "capability");
assert.equal(environment.normalizedKind({ kind: "dependency" }), "dependency");
assert.equal(environment.normalizedKind({ kind: "unknown" }), "other");
assert.equal(environment.visibleConfigFields({
  kind: "dependency", health: { ok: true }, config: { schema: [{ key: "path", type: "file" }] }
}, false).length, 0);
assert.equal(environment.visibleConfigFields({
  kind: "dependency", health: { ok: true }, config: { schema: [{ key: "path", type: "file" }] }
}, true).length, 1);
assert.equal(environment.visibleConfigFields({
  kind: "dependency", health: { ok: false }, config: { schema: [{ key: "path", type: "file" }] }
}, false).length, 1);
assert.equal(environment.visibleConfigFields({
  kind: "capability", health: { ok: true }, config: { schema: [{ key: "output", type: "directory" }] }
}, false).length, 1);
assert.equal(environment.editorValue({
  health: { path: "C:\\detected\\ffmpeg.exe" }, config: { values: { ffmpegPath: "C:\\configured\\ffmpeg.exe" } }
}, { key: "ffmpegPath", type: "file" }), "C:\\configured\\ffmpeg.exe");
assert.equal(environment.editorValue({
  health: { path: "C:\\detected\\ffmpeg.exe" }, config: { values: { ffmpegPath: "" } }
}, { key: "ffmpegPath", type: "file" }), "C:\\detected\\ffmpeg.exe");
assert.equal(environment.toggleMetadata({ config: { toggle: { key: "enabled", label: "启用" } } }).key, "enabled");
assert.equal(environment.toggleMetadata({ config: {} }), null);

const environmentSource = require("node:fs").readFileSync(require("node:path").join(__dirname, "../popup/environment.js"), "utf8");
assert.match(environmentSource, /navigation\.push\(\{ type: "edit", id: tool\.name \}\)/);
assert.match(environmentSource, /navigation\.pop\(\)/);
assert.match(environmentSource, /settings-heading|settingsHeading/);
assert.doesNotMatch(environmentSource, /defaultExpanded/);
assert.doesNotMatch(environmentSource, /tool\.name === "go-downloader"/);
assert.doesNotMatch(environmentSource, /dependencyNeedsAttention|hasRecoveryAction/);

const backgroundSource = require("node:fs").readFileSync(require("node:path").join(__dirname, "../background.js"), "utf8");




assert.match(backgroundSource, /name: "media-sniffer"/);
assert.match(backgroundSource, /const result = await bridgeFetch\("\/v1\/tools"\)/);
assert.match(backgroundSource, /function localServiceTool\(bridge\)/);
assert.match(backgroundSource, /displayName: "本地服务"/);
assert.doesNotMatch(backgroundSource, /function nativeHostTool\(/);
assert.doesNotMatch(backgroundSource, /name: "native-host"/);
assert.match(backgroundSource, /offlineBridgeTool\("浏览器原生连接失败"/);
assert.match(backgroundSource, /offlineBridgeTool\("下载服务连接失败"/);
assert.match(backgroundSource, /key: "detectHls"/);
assert.match(backgroundSource, /key: "detectDash"/);
assert.match(backgroundSource, /key: "detectMp4"/);
assert.doesNotMatch(backgroundSource, /MEDIA_SNIFFER_CONFIG_KEY|legacyToolSnapshot/);
assert.match(backgroundSource, /preferencesReady = bridgeFetch\("\/v1\/preferences"\)/);
assert.match(backgroundSource, /v1\/tools\/whisper\/models\/install/);
assert.match(backgroundSource, /bridge\.logs\.list/);
assert.match(backgroundSource, /bridge\.logs\.clear/);
assert.match(backgroundSource, /ui\.logs\.set/);
assert.match(backgroundSource, /message\.action === "ui\.about"/);
assert.match(backgroundSource, /chrome\.runtime\.getManifest\(\)\.version/);
assert.match(backgroundSource, /bridgeFetch\("\/v1\/environment"\)/);

assert.match(backgroundSource, /v1\/tools\/llama\/models\/install/);
assert.match(backgroundSource, /model: String\(message\.model/);
assert.match(environmentSource, /requiresLicenseAcceptance/);
assert.match(environmentSource, /acceptLicense: requiresAcceptance/);
assert.match(environmentSource, /field\.type === "boolean"/);
assert.match(environmentSource, /tool\.config\.scope === "extension"/);

console.log("extension environment tests passed");
