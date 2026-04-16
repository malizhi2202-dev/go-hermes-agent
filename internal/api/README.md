# internal/api

这个目录提供 Go 版本的 HTTP API。

当前负责：

- 登录鉴权接口
- chat / context / history / search / audit
- models / memory / tools / extensions
- multiagent 的 `plan / run / traces / replay / resume`

设计原则：

- API 只做鉴权、参数校验和响应序列化
- 业务逻辑尽量下沉到 `internal/app`
- gateway webhook 和业务 API 共用同一套应用容器
