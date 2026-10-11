#!/usr/bin/env node
// Optional browser acceptance harness. Puppeteer is an external, mature SDK:
// no Puppeteer/Chromium binaries are bundled in the DownKit release.
import { parseArgs } from 'node:util';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { mkdtemp, rm, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';

const here = dirname(fileURLToPath(import.meta.url));
const projectRoot = resolve(here, '..');
const { values } = parseArgs({
  options: {
    puppeteer: { type: 'string' },
    chrome: { type: 'string' },
    extension: { type: 'string' },
    output: { type: 'string' },
    mode: { type: 'string', default: 'smoke' },
    'inject-failure': { type: 'boolean', default: false },
    'close-timeout-ms': { type: 'string', default: '5000' },
    'hold-ms': { type: 'string', default: '0' },
  },
  strict: true,
});
const closeTimeout = Number(values['close-timeout-ms']);
if (!Number.isInteger(closeTimeout) || closeTimeout < 500 || closeTimeout > 30000) {
  throw new Error('--close-timeout-ms must be 500–30000');
}
const holdMS = Number(values['hold-ms']);
if (!Number.isInteger(holdMS) || holdMS < 0 || holdMS > 30000) throw new Error('--hold-ms must be 0–30000');
if (!['smoke', 'popup'].includes(values.mode)) throw new Error('--mode must be smoke or popup');
if (!values.chrome) throw new Error('--chrome is required (a Chromium executable)');
const require = createRequire(import.meta.url);
let puppeteer;
try {
  puppeteer = require(values.puppeteer ? resolve(values.puppeteer) : 'puppeteer');
} catch (error) {
  throw new Error('Puppeteer is required for this optional developer tool; pass --puppeteer /absolute/path/to/puppeteer (no project dependency is added)', { cause: error });
}

const profileDir = await mkdtemp(join(tmpdir(), 'downkit-browser-acceptance-'));
const extensionDir = resolve(values.extension ?? join(projectRoot, 'extension'));
const start = performance.now();
let browser;
let browserPID;
let issue;
let shutdownMode = 'graceful';
let closing;
const abort = new AbortController();
function closeOwnedBrowser() {
  return closing ??= browser.close();
}
for (const name of ['SIGINT', 'SIGTERM']) {
  process.once(name, () => {
    issue ??= new Error('Interrupted by ' + name);
    abort.abort();
    if (browser) void closeOwnedBrowser().catch(() => browser.process()?.kill('SIGKILL'));
  });
}
try {
  browser = await puppeteer.launch({
    headless: true,
    enableExtensions: values.mode === 'popup',
    executablePath: resolve(values.chrome),
    userDataDir: profileDir,
    args: ['--no-sandbox', '--disable-setuid-sandbox', ...(values.mode === 'popup'
      ? ['--disable-extensions-except=' + extensionDir, '--load-extension=' + extensionDir] : [])],
  });
  browserPID = browser.process()?.pid;
  if (values['inject-failure']) throw new Error('Injected failure immediately after Chromium launch');

  const page = await browser.newPage();
  try {
    await page.setViewport({ width: 520, height: 740, deviceScaleFactor: 1 });
    if (values.mode === 'popup') {
      const target = await browser.waitForTarget((candidate) => candidate.url().startsWith('chrome-extension://'), { timeout: 10000 });
      const extensionID = new URL(target.url()).hostname;
      await page.goto('chrome-extension://' + extensionID + '/popup/popup.html', { waitUntil: 'domcontentloaded', timeout: 10000 });
    } else {
      await page.goto('about:blank', { timeout: 10000 });
    }
    if (holdMS) await sleep(holdMS, undefined, { signal: abort.signal });
    if (values.output) {
      const output = resolve(values.output);
      await mkdir(dirname(output), { recursive: true });
      await page.screenshot({ path: output, fullPage: true });
    }
  } finally {
    await page.close().catch(() => {});
  }
} catch (error) {
  issue = error;
} finally {
  if (browser) {
    let timer;
    try {
      await Promise.race([
        closeOwnedBrowser(),
        new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('Chromium close deadline exceeded')), closeTimeout); }),
      ]);
    } catch (error) {
      shutdownMode = 'force-kill-owned-browser';
      browser.process()?.kill('SIGKILL'); // only this harness's owned browser, never a global pkill
      issue ??= error;
    } finally {
      clearTimeout(timer);
    }
  }
  // The isolated profile is owned by this invocation; do not delete user profiles.
  await rm(profileDir, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }).catch((error) => { issue ??= error; });
  console.log(JSON.stringify({
    event: 'browser_acceptance_completed',
    status: issue ? 'failed' : 'ok',
    mode: values.mode,
    browser_pid: browserPID ?? null,
    shutdown: shutdownMode,
    elapsed_ms: Math.round(performance.now() - start),
    error: issue ? issue.message : null,
  }));
}
if (issue) process.exitCode = 1;
