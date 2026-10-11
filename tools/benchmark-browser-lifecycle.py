#!/usr/bin/env python3
"""Reproducible failing-browser A/B, scoped to processes launched by this script.

A reproduces the former catch-without-finally pattern. The watchdog cleans up
only its own Node/Chromium process groups, never existing sessions.
B uses tools/browser-acceptance.mjs's isolated-profile/finally cleanup.
"""
import argparse
import json
import os
import re
import signal
import statistics
import subprocess
import sys
import time
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument("--puppeteer", required=True, help="Absolute Puppeteer module path")
parser.add_argument("--chrome", required=True, help="Absolute Chromium executable")
parser.add_argument("--repeats", type=int, default=3)
parser.add_argument("--deadline", type=float, default=4.0)
args = parser.parse_args()
if not 1 <= args.repeats <= 10 or not 2 <= args.deadline <= 20:
    parser.error("repeats must be 1..10; deadline 2..20s")
node = "node"
runner = str(Path(__file__).with_name("browser-acceptance.mjs"))
legacy = r"""
const puppeteer=require(process.argv[1]);
(async()=>{
 const browser=await puppeteer.launch({
   headless:true, executablePath:process.argv[2],
   args:['--no-sandbox','--disable-setuid-sandbox'],
 });
 console.log('BROWSER_PID='+browser.process().pid);
 throw Error('injected failure after successful launch');
})().catch(err=>{console.error(err.message);process.exitCode=1});
// This is the deliberately broken old behavior: browser.close() is missing.
"""
def alive(pid):
    try:
        with open(f"/proc/{pid}/stat", encoding="utf-8") as stream:
            return stream.read().split()[2] not in ("Z", "X")
    except (FileNotFoundError, ProcessLookupError):
        return False

def owned_cleanup(node_pid, chrome_pid):
    # Puppeteer starts Chromium in a separate process group on Linux.
    for pid in (chrome_pid, node_pid):
        if not pid:
            continue
        try:
            group = os.getpgid(pid)
            if group == pid: os.killpg(group, signal.SIGKILL)
            else: os.kill(pid, signal.SIGKILL)
        except (ProcessLookupError, PermissionError): pass

results = []
for label in ("legacy", "finally"):
    for iteration in range(args.repeats):
        command = ([node, "-e", legacy, args.puppeteer, args.chrome] if label == "legacy" else
                   [node, runner, "--puppeteer", args.puppeteer, "--chrome", args.chrome, "--inject-failure"])
        start = time.monotonic()
        process = subprocess.Popen(command, start_new_session=True, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, text=True, encoding="utf-8",
                                   errors="replace", env=os.environ.copy())
        timed_out = False
        stdout, stderr = "", ""
        chrome_pid = None
        try:
            try:
                stdout, stderr = process.communicate(timeout=args.deadline)
            except subprocess.TimeoutExpired:
                timed_out = True
                # Pipes may remain open because Chromium inherited descriptors.
                # First kill own groups; then drain safely.
                try:
                    stdout_data = process.stdout.read(0) if process.stdout else ""
                except Exception:
                    stdout_data = ""
                # On timeout, browser PID may be present but stdout is buffered in pipe.
                # A nonblocking read obtains it without waiting for process termination.
                if process.stdout:
                    os.set_blocking(process.stdout.fileno(), False)
                    try: stdout += process.stdout.read() or ""
                    except (BlockingIOError, TypeError): pass
                match = re.search(r"BROWSER_PID=(\d+)", stdout)
                chrome_pid = int(match.group(1)) if match else None
                owned_cleanup(process.pid, chrome_pid)
                try:
                    tailout, tailerr = process.communicate(timeout=3)
                    stdout += tailout or ""
                    stderr += tailerr or ""
                except subprocess.TimeoutExpired:
                    owned_cleanup(process.pid, chrome_pid)
        finally:
            if chrome_pid is None:
                match = re.search(r"BROWSER_PID=(\d+)", stdout)
                if match: chrome_pid = int(match.group(1))
                else:
                    for line in stdout.splitlines():
                        try:
                            value = json.loads(line)
                            chrome_pid = value.get("browser_pid")
                        except ValueError: continue
            still_alive_before_cleanup = alive(chrome_pid) if chrome_pid else False
            owned_cleanup(process.pid, chrome_pid)
            if process.poll() is None:
                try: process.wait(timeout=3)
                except subprocess.TimeoutExpired: process.kill()
        duration = round((time.monotonic() - start) * 1000)
        event = dict(variant=label, trial=iteration + 1, elapsed_ms=duration,
                     deadline_exceeded=timed_out, browser_pid=chrome_pid,
                     browser_alive_before_cleanup=still_alive_before_cleanup,
                     exited_code=process.returncode)
        results.append(event)
        print("SAMPLE", json.dumps(event, sort_keys=True), flush=True)

for label in ("legacy", "finally"):
    subset = [x for x in results if x["variant"] == label]
    print("SUMMARY", json.dumps(dict(
        variant=label,
        trials=len(subset),
        median_ms=round(statistics.median(x["elapsed_ms"] for x in subset)),
        watchdog_timeouts=sum(x["deadline_exceeded"] for x in subset),
        browser_alive_before_cleanup=sum(x["browser_alive_before_cleanup"] for x in subset),
    ), sort_keys=True), flush=True)
