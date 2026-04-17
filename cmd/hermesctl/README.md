# cmd/hermesctl

`hermesctl` 是 Go 版 Hermes-Agent 的轻量控制台入口。

设计目标：

- 用 CLI 覆盖主要的管理、排障和运维能力
- 尽量复用 `internal/app`、`internal/store`、`internal/extensions`、`internal/multiagent` 的现成契约
- 保持单文件入口、易部署、易理解

当前命令覆盖：

- 认证与会话：`init-admin`、`login`、`chat`、`context`、`prompt-inspect`、`prompt-cache-stats`、`prompt-cache-clear`、`prompt-config`
- 模型：`models`、`discover-models`、`switch-model`、`model-metadata`
- 辅助模型：`auxiliary-info`、`auxiliary-chat`、`auxiliary-switch`
- 批处理与轨迹：`batch-run`、`trajectories`、`trajectory-summary`、`trajectory-show`
- 定时任务：`cron-add`、`cron-list`、`cron-show`、`cron-delete`、`cron-tick`
- 历史与审计：`sessions`、`history`、`search`、`audit`、`execution-audit`、`execution-profile-audit`
- 扩展与工具：`extensions`、`extension-hooks`、`extension-refresh`、`extension-state`、`extension-validate`、`tools`、`tool-exec`
- 多 Agent：`multiagent-plan`、`multiagent-run`、`multiagent-traces`、`multiagent-summary`、`multiagent-verifiers`、`multiagent-failures`、`multiagent-hotspots`、`multiagent-replay`、`multiagent-resume`

统一交互式控制台：

- `hermesctl chat` 在交互模式下现在不只是聊天页，也是一体化控制台
- 普通文本输入继续走聊天
- slash commands 现在支持：
  - `/login`
  - `/whoami`
  - `/models`
  - `/model`
  - `/context`
  - `/sessions`
  - `/history`
  - `/search`
  - `/audit`
  - `/extensions`
  - `/tools`
  - `/multiagent-plan`
  - `/multiagent-run`
  - `/multiagent-replay`
  - `/trajectories`
  - `/trajectory-summary`
  - `/cron-add`
  - `/cron-list`
  - `/cron-show`
  - `/cron-delete`
  - `/cron-tick`
  - `/help`
  - `/exit`

方案说明：

- 参数解析使用标准库 `flag`，减少依赖和学习成本。
- 复杂返回统一走 pretty JSON，方便人看，也方便 shell 管道和其他自动化工具消费。
- CLI 尽量镜像 HTTP API 的输入输出契约，这样 Web/API/CLI 三条链共用一套领域模型，后续维护成本更低。
- 多 Agent 相关命令采用 `tasks-file` / `plan-file` 方式，避免在终端里拼复杂 JSON，降低误用风险。
- Batch/trajectory 采用 JSONL 输入输出，方便和后续训练、评估、自动化脚本直接衔接。
- `batch-run --resume` 通过 run name 对应的 checkpoint 文件继续处理，适合单机长任务恢复。
- `trajectories` 和 `trajectory-summary` 支持轻量过滤，便于直接在 CLI 里做排障和数据盘点。
- cron 相关命令采用 JSON 文件持久化和显式 `tick`，既能让 hermesd 后台自动调度，也方便通过 CLI 手动排障。
- 统一交互控制台让最常见的管理动作可以在同一个登录态里完成，减少反复输入长命令的成本。
