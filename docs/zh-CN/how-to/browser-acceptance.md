# 浏览器验收与进程生命周期

DownKit 的可选浏览器验收程序复用成熟的 **Puppeteer + Chromium**，不重新实现浏览器，也不将它们加入正式安装包。每次启动使用独立临时 Profile，正常结束、异常和 SIGTERM 中断时都会回收浏览器。

## 如何运行

在仓库根目录执行：

```bash
node tools/browser-acceptance.mjs \
  --puppeteer /absolute/path/to/node_modules/puppeteer \
  --chrome /absolute/path/to/chrome \
  --mode popup \
  --output /tmp/downkit-popup.png
```

`--mode smoke` 只验证 Chromium 启动和退出；`--mode popup` 真实加载 DownKit 扩展并截图。**截图不能证明 Native Messaging 已连通，也不代表下载成功。** 浏览器、Puppeteer 依赖自行安装；如 WSL 缺少 Chromium 共享库，需要先安装系统库或在测试环境设置 `LD_LIBRARY_PATH`。

异常处理优先调用 Puppeteer `browser.close()`；关闭超时只终止本次程序拥有的浏览器进程。SIGINT/SIGTERM 进入同一清理流程，不使用全局 `pkill chrome`，也不重启 Desktop Commander。

## 可复现 A/B 基准

```bash
python3 tools/benchmark-browser-lifecycle.py \
  --puppeteer /absolute/path/to/node_modules/puppeteer \
  --chrome /absolute/path/to/chrome \
  --repeats 3 --deadline 4
```

基准 A 模拟旧脚本仅在 `catch` 设置退出码、没有在 `finally` 执行 `browser.close()` 的行为；B 使用新的验收工具。A 的看门狗仅清理本次基准启动的 Node/Chrome 进程组，不影响现有浏览器。

2026-10-11 WSL 实测：

| 指标 | 旧异常处理 | 新 `finally` |
| --- | ---: | ---: |
| 次数 | 3 | 3 |
| 超过 4 秒看门狗期限 | 3/3 | 0/3 |
| 检测时浏览器仍存活 | 3/3 | 0/3 |
| 退出耗时中位数 | ≥4,015 ms（测试期限截断） | 423 ms |

额外通过了真实 SIGTERM 中断测试：信号到退出约 33 ms，Chromium 已退出；`popup` 模式也成功生成 520×740 PNG 并清理浏览器。**这些数字仅针对异常与取消路径，不代表正常页面加载提速。**

## DevTool 与 Desktop Commander 的职责

DownKit 已通过配置接入 DV2 现有 Git Workspace Provider，新增 `workspace_create/list/inspect/remove`，不自研 Git Worktree 管理。开发中优先复用一个持久 DevTool MCP 会话的 `project_verify`、`project_build`、`scm_publish` 等能力；Desktop Commander 继续用于远程访问和必要的 GUI 操作。其现有详细日志保留不变。

参见 [DevTool 架构](../architecture/devtool.md)。
