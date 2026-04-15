# Go 版本架构总结

## 总体定位

Go 版本不是对 Python 项目的逐行翻译，而是把原仓库重构为一条“安全主干”：

1. 配置加载
2. 安全登录与鉴权
3. 会话与审计存储
4. LLM 调用边界
5. API / Gateway 接入
6. 受限工具系统
7. 受控动态扩展

它的目标是先提供一个可运行、可安装、可审计、可继续迁移的基础平台。

## 核心架构分层

### 1. 入口层

- `cmd/hermesd`
  - 服务端启动入口
  - 加载配置
  - 初始化 `App`
  - 启动 HTTP API

- `cmd/hermesctl`
  - 管理命令入口
  - 初始化管理员
  - 登录并获取 token
  - 输出版本信息

### 2. 应用装配层

- `internal/app`
  - 负责把配置、存储、鉴权、LLM、工具、扩展、执行器装配成统一应用对象 `App`
  - 所有 API 和 Gateway 都通过 `App` 进入核心能力

### 3. 配置与安全层

- `internal/config`
  - 负责加载 `config.yaml`
  - 管理 LLM、Gateway、Execution、Extensions、MCP 配置

- `internal/auth`
  - 负责本地账号登录
  - 密码校验
  - JWT 签发与解析

- `internal/security`
  - 密码哈希
  - token 工具

### 4. 数据与状态层

- `internal/store`
  - SQLite 持久化
  - `users`
  - `sessions`
  - `messages`
  - `messages_fts`
  - `audit_log`
  - `processed_gateway_updates`
  - `extension_states`

这是 Go 版本的共享数据底座。

### 5. Agent 运行边界层

- `internal/llm`
  - OpenAI 兼容的聊天调用边界
  - 当前承担“提示词 -> 模型响应”的最小闭环

- `internal/api`
  - 提供登录、聊天、历史、搜索、审计、工具、扩展接口

- `internal/gateway`
  - 提供基础 webhook
  - 提供 Telegram webhook 适配
  - 负责平台消息接入和去重

### 6. 工具与动态扩展层

- `internal/tools`
  - 注册内置白名单工具
  - 执行工具调用

- `internal/extensions`
  - 扫描 `plugin.yaml`
  - 扫描 `SKILL.md + skill.yaml`
  - 扫描 `mcp_servers`
  - 注册 `plugin.*`
  - 注册 `skill.*`
  - 注册 `mcp.*`
  - 管理扩展启停状态

### 7. 高风险能力收口层

- `internal/execution`
  - 只提供受控命令执行骨架
  - 默认关闭
  - 白名单命令
  - 参数限制
  - 输出限制
  - 审计记录

## 版本特征

Go 版本当前有三个明显特征：

1. 安全优先
   - 高风险动态能力不直接开放
   - 所有扩展能力先变成受控边界

2. 模块收敛
   - Python 的多入口、多动态注册点，被收敛到 `App + API + Extensions`

3. 易于继续迁移
   - 已经给后续迁移预留了 Gateway、Extensions、Execution 三条扩展线

## 与 Python 版本的关键差异

- Python 偏运行时动态组装，Go 偏显式结构化装配
- Python 的插件、skills、MCP 更自由，Go 版本先做可治理接入
- Python 有更强的工具链广度，Go 版本先保证安全闭环和运行骨架

## 当前 Go 主调用主线

1. `cmd/hermesd` 启动
2. 读取配置
3. `app.New()` 初始化 Store/Auth/LLM/Tools/Extensions/Execution
4. API 或 Gateway 收到请求
5. JWT 鉴权
6. 进入聊天、搜索、审计、工具、扩展等能力
7. 结果写入 SQLite 和 Audit

## 当前适合承担的职责

- 安全 API 服务
- 安全登录与本地部署
- 会话与历史检索
- Telegram / webhook 基础消息接入
- 受控工具执行
- 受控插件 / skill / MCP 扩展接入

## 当前仍保留给后续迁移的能力

- 更完整的多轮 agent 上下文编排
- Slack / 更多平台 gateway
- 更完整的 MCP transport
- 热插拔 plugin hook 生命周期
- 浏览器自动化
- 更复杂的代理/委派执行链
