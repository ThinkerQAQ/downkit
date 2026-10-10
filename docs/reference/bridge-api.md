# Bridge HTTP API

Desktop Bridge binds to `127.0.0.1:17891`. Privileged endpoints require `X-DownKit-Token`, delivered to the browser extension through Native Messaging.

| Endpoint | Purpose |
| --- | --- |
| `GET /v1/ping` | Health |
| `GET /v1/environment` | Runtime status |
| `GET/PUT /v1/config` | Configuration |
| `GET /v1/tools` | Tool registry |
| `POST /v1/tasks` | Submit download |
| `POST /v1/playlist/probe` | Inspect playlist |
| `GET /v1/jobs` | Jobs |
| `/v1/jobs/{id}/…` | Job actions |
| `POST /v1/open-output` | Open output directory |
| `GET /console/` | Local UI assets |

Web UI calls traverse the browser extension relay and cannot directly obtain the token.
