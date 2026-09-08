# AI Recommend

Suggests titles to add from the current library: sequels, franchise gaps, and genre neighbors. Offline catalog; optional `ai-runtime` complete calls can refine reasons later.

## Ports

| Service | Default |
|---------|---------|
| gRPC | `127.0.0.1:9764` |
| HTTP | `127.0.0.1:9765` |

`POST /v1/suggest` with `{ "library": [ { "title", "genres", "franchise" } ] }`.
