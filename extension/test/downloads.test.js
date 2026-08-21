"use strict";

const assert = require("node:assert/strict");
global.DownKitPopup = {};
const downloads = require("../popup/downloads.js");

assert.equal(downloads.selectedQuality({ getElementById: () => null }), "");
assert.equal(downloads.selectedQuality({ getElementById: () => ({ value: "1080" }) }), "1080");

const source = require("node:fs").readFileSync(require("node:path").join(__dirname, "../popup/downloads.js"), "utf8");
assert.match(source, /activateTab\("jobs"\)/);
assert.match(source, /media\.playlist\.probe/);
assert.match(source, /playlist\s*\n\s*\}/);
assert.match(source, /navigator\.clipboard\.writeText/);
assert.match(source, /DownKitMedia\.resourceName/);

console.log("extension downloads tests passed");
