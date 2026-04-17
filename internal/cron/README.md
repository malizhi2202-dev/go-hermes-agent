# internal/cron

`internal/cron` 是 Go 版对 Python `cron/jobs.py` 和 `cron/scheduler.py` 的轻量迁移。

当前职责：

- 管理单机 cron jobs
- 解析轻量 schedule
- 按到期时间执行 job
- 把输出保存到 `data/cron/output/`
- 支持 hermesd 后台自动 tick
- 支持 CLI 手动管理和手动 tick

当前设计取舍：

- job 持久化先使用 `data/cron/jobs.json`
- 先支持 `30m`、`2h`、`1d`、`every 30m`、RFC3339 时间
- 先不迁复杂平台投递和 cron 表达式依赖
- 优先保证单机易部署、易排障、易理解

这样做的优势：

- 不需要额外数据库 schema 和第三方 scheduler
- 用户能直接查看 job 文件和输出文件
- hermesctl 和 hermesd 共用同一套轻量调度主干
