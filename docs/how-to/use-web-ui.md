# Use the Web UI

1. Install the desktop release and browser extension.
2. Click **Open workspace** in the extension header.
3. The browser opens `http://127.0.0.1:17891/console/`.

The workspace shares extension features but depends on the installed extension for authenticated actions. The Bridge binds to loopback; the page never receives its token. Reload the unpacked extension after changing its source.

The Settings page uses Android-style drill-down navigation: **Settings → Category → Tool details → Edit**. Language and extension open mode are under **Settings → Appearance & interaction**. The Web UI uses an icon sidebar; extension surfaces use compact icon navigation.

Popup and Side Panel use an icon rail, while Web UI uses a labelled sidebar. The detection page groups page context, download options and detected resources into one consistent card.

## View runtime logs

Open the **Logs** item in the sidebar to inspect Bridge and Worker events. Search, filter by severity, auto-refresh, select, copy or clear the visible logs. The navigation item can be hidden in **Settings → Appearance & interaction → Show logs tab**; the files remain under `Data/logs/`.
