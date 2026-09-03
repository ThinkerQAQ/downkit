"use strict";

const assert = require("node:assert/strict");
global.DownKitPopup = {};
const downloads = require("../popup/downloads.js");

function controls(values) {
  return { getElementById: id => Object.hasOwn(values, id) ? { value: values[id] } : null };
}

assert.equal(downloads.selectedQuality({ getElementById: () => null }), "");
assert.equal(downloads.selectedQuality({ getElementById: () => ({ value: "1080" }) }), "1080");
assert.deepEqual(downloads.selectedSubtitleRequest(controls({ subtitleMode: "site-zh" })), {
  mode: "site", languages: ["zh.*", "zh-Hans", "zh-Hant"], includeAutomatic: true, format: "best"
});
assert.deepEqual(downloads.selectedSubtitleRequest({ getElementById: () => null }), { mode: "none" });
assert.deepEqual(downloads.selectedSubtitleRequest(controls({ subtitleMode: "asr", asrLanguage: "en" })), {
  mode: "asr", includeAutomatic: false, format: "srt", asrLanguage: "en"
});
assert.deepEqual(downloads.selectedSubtitleRequest(controls({
  subtitleMode: "site-or-asr", asrLanguage: "ja", subtitleTargetLanguage: "zh-Hans", subtitleLayout: "bilingual"
})), {
  mode: "site-or-asr", languages: ["ja.*", "ja"], includeAutomatic: true, format: "best",
  asrLanguage: "ja", targetLanguage: "zh-Hans", bilingual: true
});
assert.throws(() => downloads.selectedSubtitleRequest(controls({
  subtitleMode: "asr", asrLanguage: "auto", subtitleTargetLanguage: "zh-Hans"
})), /明确选择视频语音/);

const visibilityElements = {
  subtitleMode: { value: "asr" },
  asrLanguageLabel: { hidden: true },
  asrLanguage: { hidden: true },
  asrLanguageHint: { hidden: true },
  subtitleTargetLanguageLabel: { hidden: true },
  subtitleTargetLanguage: { hidden: true, value: "" },
  subtitleTargetLanguageHint: { hidden: true },
  subtitleLayoutLabel: { hidden: true },
  subtitleLayout: { hidden: true },
  subtitleLayoutHint: { hidden: true }
};
const visibilitySource = { getElementById: id => visibilityElements[id] || null };
assert.deepEqual(downloads.syncSubtitleOptionsVisibility(visibilitySource), {
  subtitlesVisible: true, asrVisible: true, layoutVisible: false
});
assert.equal(visibilityElements.asrLanguage.hidden, false);
assert.equal(visibilityElements.subtitleTargetLanguage.hidden, false);
visibilityElements.subtitleTargetLanguage.value = "zh-Hans";
assert.equal(downloads.syncSubtitleOptionsVisibility(visibilitySource).layoutVisible, true);
assert.equal(visibilityElements.subtitleLayout.hidden, false);
visibilityElements.subtitleMode.value = "site-en";
assert.equal(downloads.syncSubtitleOptionsVisibility(visibilitySource).asrVisible, false);
assert.equal(visibilityElements.asrLanguageHint.hidden, true);
visibilityElements.subtitleMode.value = "none";
assert.equal(downloads.syncSubtitleOptionsVisibility(visibilitySource).subtitlesVisible, false);
assert.equal(visibilityElements.subtitleTargetLanguage.hidden, true);

const source = require("node:fs").readFileSync(require("node:path").join(__dirname, "../popup/downloads.js"), "utf8");
assert.match(source, /activateTab\("jobs"\)/);
assert.match(source, /media\.playlist\.probe/);
assert.match(source, /playlist,\s*\n\s*subtitles/);
assert.match(source, /navigator\.clipboard\.writeText/);
assert.match(source, /DownKitMedia\.resourceName/);

console.log("extension downloads tests passed");
