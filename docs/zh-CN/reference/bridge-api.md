# Bridge API

Bridge 监听 `127.0.0.1:17891`。受保护接口要求 `X-DownKit-Token`，由 Native Messaging 交给浏览器扩展后台。

| 路径 | 用途 |
| --- | --- |
| `GET /v1/ping` | 健康检查 |
| `GET /v1/environment` | 环境 |
| `GET/PUT /v1/config` | 设置 |
| `GET /v1/tools` | 工具状态 |
| `POST /v1/tasks` | 创建下载任务 |
| `POST /v1/playlist/probe` | 播放列表预检 |
| `GET /v1/jobs` | 查询任务 |
| `/v1/jobs/{id}/…` | 任务操作 |
| `POST /v1/open-output` | 打开输出目录 |
| `GET /console/` | 本机工作台 |

Web UI 通过扩展中继调用有权限的接口，不能直接取得 Token。
