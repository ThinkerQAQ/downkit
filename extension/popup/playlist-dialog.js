(function (root) {
  "use strict";

  function choose(result) {
    if (!result || !result.playlist || Number(result.count) <= 1) {
      return Promise.resolve("single");
    }

    const dialog = document.getElementById("playlistDialog");
    const count = Number(result.count);
    dialog.querySelector(".playlist-count").textContent = String(count);
    dialog.querySelector(".playlist-all").textContent = `下载全部 ${count} 个视频`;
    dialog.returnValue = "";

    return new Promise(resolve => {
      const closed = () => {
        dialog.removeEventListener("close", closed);
        resolve(dialog.returnValue === "all" ? "all" : dialog.returnValue === "single" ? "single" : null);
      };
      dialog.addEventListener("close", closed);
      dialog.showModal();
    });
  }

  root.DownKitPlaylistDialog = { choose };
  if (typeof module !== "undefined" && module.exports) module.exports = { choose };
})(globalThis);
