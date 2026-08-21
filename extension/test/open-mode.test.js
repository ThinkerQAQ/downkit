"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const manifest = JSON.parse(fs.readFileSync(path.join(__dirname, "../manifest.json"), "utf8"));
const background = fs.readFileSync(path.join(__dirname, "../background.js"), "utf8");

assert.equal(manifest.action.default_popup, "popup/popup.html");
assert.equal(manifest.side_panel.default_path, "popup/popup.html");
assert.match(background, /const DEFAULT_OPEN_MODE = "popup"/);
assert.match(background, /setPopup\(\{ popup: mode === "popup" \? POPUP_PATH : "" \}\)/);
assert.match(background, /openPanelOnActionClick: mode === "sidePanel"/);
assert.match(background, /chrome\.storage\.local\.set\(\{ openMode: mode \}\)/);

console.log("extension open mode tests passed");
