# Python -> Go 模块映射与迁移决策

## 迁移原则

- 不做逐文件机械翻译
- 先保留主干能力
- 动态高风险能力先降级或隔离
- 明确“保持 / 调整 / 延期”

## 模块映射

| Python 模块 | Go 模块 | 状态 |
| --- | --- | --- |
| `run_agent.py` | `internal/app`, `internal/llm`, `internal/api` | 调整后迁移 |
| `hermes_state.py` | `internal/store` | 已开始迁移 |
| `hermes_cli/auth.py` | `internal/auth`, `internal/security` | 已开始迁移 |
| `hermes_constants.py` | `internal/config` | 已开始迁移 |
| `hermes_cli/main.py`, `cli.py` | `cmd/hermesctl` | 已开始迁移 |
| `gateway/run.py` | `cmd/hermesd`, `internal/api` | 已开始迁移 |
| `toolsets.py`, `model_tools.py` | 后续 `internal/tools` | 延期 |
| `tools/terminal_tool.py` | 不直接迁移 | 延期且需人工审核 |
| `tools/code_execution_tool.py` | 不直接迁移 | 高风险，暂缓 |
| `tools/browser_tool.py` | 不直接迁移 | 高风险，暂缓 |
| `tools/delegate_tool.py` | 不直接迁移 | 高风险，暂缓 |
| `gateway/platforms/*` | 后续 `internal/platforms` | 延期 |
| `plugins/*` | 后续受限插件接口 | 延期 |
| `skills/*` | 先搬迁文档，不执行脚本 | 部分迁移 |

## 行为一致性说明

### 保持

- 配置驱动启动
- 本地状态存储
- LLM 调用边界
- 会话与用户概念
- 可安装和卸载

### 调整

- Python 的多 provider 认证，收敛为本地安全登录 + JWT + 受控 LLM provider 配置
- 动态工具发现，后续改为编译期注册或白名单注册

### 延期

- Gateway 多平台消息适配
- 浏览器和终端执行
- Skills 脚本执行
- 子代理委派
- RL / benchmark

## 安全清理

Go 版本已经主动去掉或暂缓以下高风险内容：

- 任意 shell 执行
- 任意代码执行
- 弱边界动态加载
- 外部脚本直接运行
- 未鉴权的敏感管理接口

保留但加固：

- LLM 调用：要求通过配置显式声明
- SQLite 状态：放到受控数据目录
- 登录：bcrypt + JWT + 登录失败限制
