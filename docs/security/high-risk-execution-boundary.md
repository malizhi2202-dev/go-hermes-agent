# 高风险动态执行链边界

当前 Go 版本没有开放：

- 任意 shell
- 任意脚本执行
- 动态浏览器执行
- 子代理并发委派

代码层面已经加入默认拒绝策略：

- `internal/execution/policy.go`
- `internal/execution/executor.go`

默认行为：

- `Enabled = false`
- 未完成人工审核、沙箱、白名单、资源限制前，一律拒绝

## 受控版执行链

当前只提供受控骨架，不默认开放：

- 显式配置开关
- 命令白名单
- 单次执行超时
- 无 shell 拼接
- 仅通过 `exec.CommandContext` 直接执行二进制
- 参数数量限制
- 参数长度限制
- shell 元字符拦截
- 审计日志：attempt / denied / success
- 审计查询：`/v1/audit` 与 `/v1/audit/execution`

这意味着 Go 迁移当前是“先把能力边界立住”，而不是“先开放再补安全”。
