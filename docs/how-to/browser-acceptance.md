# Browser acceptance and lifecycle

DownKit's optional browser acceptance harness uses **Puppeteer and Chromium**. It is not a second build, installer, or application runtime. It isolates every browser profile and always closes the browser, including failed assertions and SIGTERM cancellation. The browser and Puppeteer executables must be installed separately. Nothing is added to production dependencies.

## Run locally

From the repository root:

```bash
node tools/browser-acceptance.mjs \
  --puppeteer /absolute/path/to/node_modules/puppeteer \
  --chrome /absolute/path/to/chrome \
  --mode popup \
  --output /tmp/downkit-popup.png
```

Use `--mode smoke` to launch/close Chromium without loading the extension, or `--mode popup` to load the extension and capture its initial popup. Screenshots demonstrate initial UI only, not successful native messaging, media detection, or download. Use a temporary output path; the harness creates and removes its own isolated browser profile. On systems missing Chromium shared libraries, install the distro dependencies or configure `LD_LIBRARY_PATH` for your test environment.

**Failure and cancellation behavior:** the harness calls the SDK's `browser.close()` in `finally`. On shutdown timeout it terminates **only its owned browser process**. SIGINT/SIGTERM abort ongoing waits and invoke the same close path. Do not use `pkill chrome` or stop Desktop Commander to clean up this test.

## Benchmark: intentionally broken legacy vs managed lifecycle

```bash
python3 tools/benchmark-browser-lifecycle.py \
  --puppeteer /absolute/path/to/node_modules/puppeteer \
  --chrome /absolute/path/to/chrome \
  --repeats 3 --deadline 4
```

The baseline reproduces the former `catch(() => {process.exitCode = 1})` without `browser.close()` pattern. It **only** launches test-owned browser processes, monitors them and forcibly cleans up those precise process groups after the deadline. Do not substitute a pre-existing browser PID. The improved variant runs `browser-acceptance.mjs --inject-failure` and verifies process exit and absence of leftover Chromium.

### Real WSL benchmark, October 11, 2026

| Metric | Old failure handling | Managed `finally` |
| --- | ---: | ---: |
| Trials | 3 | 3 |
| Watchdog deadline exceeded | 3/3 | 0/3 |
| Browser still alive when checked | 3/3 | 0/3 |
| Median process exit duration | ≥4,015 ms (censored by watchdog) | 423 ms |

Separately, a real SIGTERM trial while holding a browser open exited after ~33 ms from the signal; its owned Chromium process was gone. A real `--mode popup` trial generated a 520×740 PNG and exited gracefully. These timings measure **failing/cancelled lifecycle**, not page load or download throughput.

## Prefer DevTool for engineering workflows

DownKit now configures the existing DV2 Git Workspace Provider (`workspace_create/list/inspect/remove`). Use these intent-level tools, `project_verify`, `project_build` and `scm_publish` on a persistent DevTool MCP connection rather than assembling repeated remote shell commands. This does not change the requirement to launch and authorize Desktop Commander for GUI operations. Desktop Commander Remote's own verbose service logs are left unchanged.

See [DevTool architecture](../architecture/devtool.md) for how to configure DevTool and its Providers.
