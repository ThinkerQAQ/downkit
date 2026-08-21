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
assert.match(environmentSource, /tool\.config && tool\.config\.defaultExpanded/);
assert.doesNotMatch(environmentSource, /tool\.name === "go-downloader"/);
assert.doesNotMatch(environmentSource, /dependencyNeedsAttention|hasRecoveryAction/);

const backgroundSource = require("node:fs").readFileSync(require("node:path").join(__dirname, "../background.js"), "utf8");
assert.match(backgroundSource, /name: "network-proxy"/);
assert.match(backgroundSource, /key: "proxyHost"/);
assert.match(backgroundSource, /key: "proxyPort"/);
assert.match(backgroundSource, /key: "proxyEnabled"/);
assert.match(backgroundSource, /name: "media-sniffer"/);
assert.match(backgroundSource, /key: "detectHls"/);
assert.match(backgroundSource, /key: "detectDash"/);
assert.match(backgroundSource, /key: "detectMp4"/);
assert.match(backgroundSource, /MEDIA_SNIFFER_CONFIG_KEY/);
assert.match(environmentSource, /field\.type === "boolean"/);
assert.match(environmentSource, /tool\.config\.scope === "extension"/);

console.log("extension environment tests passed");
