# DownKit repository instructions

For every task that changes source code, tests, build scripts, or extension files, run this command before reporting completion:

```powershell
.\Finish-Task.cmd
```

This is the repository's required completion gate. The CMD entry point invokes `Finish-Task.ps1` with a process-scoped execution-policy bypass, runs the Go and extension test suites, and replaces `dist\downkit-windows-amd64.exe` with a newly compiled Bridge. Do not claim that a code task is complete if the command fails. Report the failure and, when the executable is locked by a running Bridge, the staged pending-build path printed by the script.

After extension UI changes, remind the user to reload the unpacked extension in `edge://extensions` or `chrome://extensions`. Rebuilding or restarting the Bridge does not reload extension JavaScript or CSS.
