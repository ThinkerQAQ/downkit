"use strict";

const assert = require("node:assert/strict");
const playlistDialog = require("../popup/playlist-dialog.js");

playlistDialog.choose({ playlist: false, count: 1 }).then(mode => {
  assert.equal(mode, "single");
  return playlistDialog.choose({ playlist: true, count: 1 });
}).then(mode => {
  assert.equal(mode, "single");
  console.log("extension playlist dialog tests passed");
}).catch(error => {
  console.error(error);
  process.exitCode = 1;
});
