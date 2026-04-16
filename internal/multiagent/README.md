# internal/multiagent

这个目录承载 Go 版本的多 Agent 编排主干。

当前职责：

- `planner`：构建执行 plan
- `policy`：限制并发、阻止冲突写入、阻止高风险工具
- `orchestrator`：按 plan 调度 task runner
- `aggregator`：汇总 child 结果、风险、下一步动作
- `types`：统一 plan / task / result / trace 结构

当前实现特点：

- 支持 `parallel` 和 `sequential`
- 支持 parent / child session 串联
- 支持结构化 `trace`
- 支持 trace summary / failure analysis
- 支持 child tool allowlist
- 支持原生 tool-calling 优先、JSON 协议回退
- 支持 replay / resume

当前未完成项：

- 递归子代理
- 深度控制链
- trace replay 后的真正 resume
- gateway 以外更多运行入口
