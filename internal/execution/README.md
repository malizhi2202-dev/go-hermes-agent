# internal/execution

这个目录负责受控执行链。

当前能力：

- 命令白名单
- 参数长度与数量限制
- 输出大小限制
- per-command rule
- 受控 profile/step 执行链
- profile 级审批
- capability token
- 失败后 rollback profile

设计原则：

- 默认关闭
- 不走 shell 拼接
- 先收缩能力面，再考虑放开

当前建议模式：

- 单命令执行：`system.exec`
- 多步受控执行：`system.exec_profile`
- profile 由配置驱动，适合固定的低风险运维/检查链
- 对高风险 profile 使用 `require_approval` 和 `capability_token`
- 对有副作用的 profile 使用 `rollback_profile`

当前观测面：

- `/v1/audit/execution`
- `/v1/audit/execution/profiles`

其中 `/v1/audit/execution/profiles` 会返回：

- `records`
- `action_summary`
- `profile_summary`

适合排查某个 `exec_profile` 的尝试、拒绝、成功和 profile 级聚合情况。
