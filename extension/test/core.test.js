"use strict";

const assert = require("node:assert/strict");
const media = require("../core/media.js");
const launcher = require("../core/launcher.js");
const cookies = require("../core/cookies.js");

assert.deepEqual(media.normalizeSnifferConfig(), { detectHls: true, detectDash: true, detectMp4: true });
assert.deepEqual(media.normalizeSnifferConfig({ detectHls: false }), { detectHls: false, detectDash: true, detectMp4: true });

assert.equal(media.classify({ url: "https://cdn.test/master.m3u8?token=1" }).kind, "hls");
assert.equal(media.classify({ url: "https://cdn.test/master.m3u8" }, { detectHls: false }), null);
assert.equal(media.classify({ url: "https://cdn.test/manifest.mpd" }, { detectDash: false }), null);
assert.equal(media.classify({ url: "https://cdn.test/video.mp4" }, { detectMp4: false }), null);
assert.equal(media.classify({ url: "https://cdn.test/video?id=1", contentType: "video/mp4; charset=binary" }).kind, "mp4");
assert.equal(media.classify({ url: "https://media.test/a/1.m4s?x=1" }).kind, "media-track");
assert.equal(media.classify({
  url: "https://r1---sn-test.googlevideo.com/videoplayback?expire=1&mime=video%2Fwebm&range=0-999"
}).kind, "media-track");
assert.equal(media.classify({
  url: "https://r1---sn-test.googlevideo.com/videoplayback?expire=1&mime=audio%2Fmp4&range=0-999"
}).kind, "media-track");
assert.equal(media.classify({ url: "https://googlevideo.example/videoplayback?mime=video%2Fmp4" }), null);
assert.equal(media.classify({ url: "https://cdn.test/segment.ts" }), null);
assert.equal(media.resourceName({ url: "https://cdn.test/path/playlist.m3u8?token=1", kind: "hls" }), "playlist.m3u8");
assert.equal(media.resourceName({ url: "https://cdn.test/video?filename=%E8%A7%86%E9%A2%91.mp4", kind: "mp4" }), "视频.mp4");
assert.equal(media.resourceName({ url: "https://cdn.test/", kind: "hls" }), "playlist.m3u8");

const page = { url: "https://page.test/watch/42", title: "标题", userAgent: "UA" };
const list = media.displayList([
  media.makeResource({ url: "https://video.cdn.test/v.m4s", tabId: 1 }),
  media.makeResource({ url: "https://audio.cdn.test/a.m4s", tabId: 1 })
], page);
assert.equal(list.length, 1);
assert.equal(list[0].kind, "resolved-media-page");
assert.equal(list[0].url, page.url);
assert.deepEqual(media.displayList([
  media.makeResource({ url: "https://cdn.test/video.mp4", tabId: 1 }),
  media.makeResource({ url: "https://cdn.test/master.m3u8", tabId: 1 })
], page, { detectMp4: false }).map(item => item.kind), ["hls"]);

const youtubePage = { url: "https://www.youtube.com/watch?v=test", title: "YouTube", userAgent: "UA" };
const youtubeList = media.displayList([
  media.makeResource({
    url: "https://r1---sn-test.googlevideo.com/videoplayback?itag=137&mime=video%2Fmp4",
    tabId: 2
  }),
  media.makeResource({
    url: "https://r1---sn-test.googlevideo.com/videoplayback?itag=140&mime=audio%2Fmp4",
    tabId: 2
  })
], youtubePage);
assert.equal(youtubeList.length, 1);
assert.equal(youtubeList[0].kind, "resolved-media-page");
assert.equal(youtubeList[0].url, youtubePage.url);

const pageCookies = [{ name: "session", value: "page", domain: "page.test", hostOnly: true, path: "/", secure: true }];
const resolvedTask = launcher.buildTask(list[0], page, "best", [], pageCookies);
assert.equal(resolvedTask.resolvePage, true);
assert.equal(resolvedTask.url, page.url);
assert.deepEqual(resolvedTask.pageCookies, pageCookies);
assert.equal(resolvedTask.playlist, "ask");

const playlistTask = launcher.buildTask(list[0], page, "best", [], pageCookies, "all");
assert.equal(playlistTask.playlist, "all");

const storedTask = launcher.buildTask(list[0], page, "best", [], pageCookies, "single", "profile");
assert.equal(storedTask.cookieStoreId, "profile");
assert.equal(storedTask.sourceTabId, 1);
assert.equal(storedTask.sourceFrameId, 0);
assert.equal(storedTask.pageUrl, page.url);

const dashTask = launcher.buildTask({
  url: "https://cdn.test/manifest?id=42",
  kind: "dash"
}, page, "best", [], pageCookies);
assert.equal(dashTask.url, page.url);
assert.equal(dashTask.resolvePage, true);

const task = launcher.buildTask({
  url: "https://cdn.test/master.m3u8?a=1&b=2",
  referer: "https://page.test/watch",
  origin: "https://page.test",
  userAgent: "Test UA",
  requestHeaders: {
    cookie: "session=secret",
    authorization: "Bearer token",
    "sec-fetch-mode": "cors",
    host: "cdn.test",
    "accept-encoding": "br"
  }
}, { title: "中文 标题", url: "https://page.test/watch" }, "best", [
  { name: "session", value: "fresh", domain: "cdn.test", hostOnly: true, path: "/", secure: true }
], pageCookies);
assert.equal(task.url, "https://cdn.test/master.m3u8?a=1&b=2");
assert.equal(task.title, "中文 标题");
assert.equal(task.quality, "best");
assert.equal(task.mediaHeaders.cookie, undefined);
assert.equal(task.mediaHeaders.authorization, "Bearer token");
assert.equal(task.mediaHeaders["sec-fetch-mode"], "cors");
assert.equal(task.pageHeaders.cookie, undefined);
assert.equal(task.mediaCookies[0].value, "fresh");
assert.deepEqual(task.pageCookies, pageCookies);

const noInventedOrigin = launcher.buildTask({
  url: "https://cdn.test/master.m3u8",
  kind: "hls",
  initiator: "https://page.test"
}, page, "", [], []);
assert.equal(noInventedOrigin.origin, "");

const sanitized = media.sanitizeRequestHeaders({ cookie: "a=b", Host: "bad", connection: "close" });
assert.deepEqual(sanitized, {});

assert.deepEqual(cookies.structured([
  { name: "session", value: "plain", path: "/" },
  { name: "__cf_bm", value: "partitioned", path: "/video" },
  { name: "__cf_bm", value: "partitioned", path: "/video" }
]).map(item => item.name), ["session", "__cf_bm"]);

const cookieQueries = [];
cookies.forURL({
  getAllCookieStores: async () => [{ id: "profile", tabIds: [7] }],
  getPartitionKey: async details => {
    assert.deepEqual(details, { tabId: 7, frameId: 3 });
    return { topLevelSite: "https://page.test" };
  },
  getAll: async details => {
    cookieQueries.push(details);
    return details.partitionKey
      ? [{ name: "__cf_bm", value: "partitioned", path: "/" }]
      : [{ name: "session", value: "plain", path: "/" }];
  }
}, "https://cdn.test/video.m3u8", 7, 3).then(structuredCookies => {
  assert.deepEqual(structuredCookies.map(item => item.name), ["session", "__cf_bm"]);
  assert.equal(cookieQueries.length, 2);
  assert.equal(cookieQueries[0].storeId, "profile");
  assert.deepEqual(cookieQueries[1].partitionKey, { topLevelSite: "https://page.test" });
  console.log("extension core tests passed");
}).catch(error => {
  console.error(error);
  process.exitCode = 1;
});
