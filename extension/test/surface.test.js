"use strict";

const assert = require("node:assert/strict");
const { detectSurface } = require("../popup/surface.js");

const popupWindow = {};
const sidePanelWindow = {};
const chromeWithPopup = {
  extension: {
    getViews(options) {
      assert.deepEqual(options, { type: "popup" });
      return [popupWindow];
    }
  }
};

assert.equal(detectSurface(chromeWithPopup, popupWindow), "popup");
assert.equal(detectSurface(chromeWithPopup, sidePanelWindow), "side-panel");
assert.equal(detectSurface(undefined, popupWindow), "popup");
assert.equal(detectSurface({ extension: { getViews() { throw new Error("unavailable"); } } }, popupWindow), "popup");

console.log("extension surface tests passed");
