# System

DownKit consists of a browser extension, a local Go Bridge and a separate Android app. The extension detects media and holds browser session context. The Go Bridge manages desktop jobs, persistence, downloads and optional tools. Android handles shared media links natively.

Popup, Side Panel and the Web UI share UI modules. The Web UI uses an extension relay for authenticated actions and never reads Native Messaging credentials.
