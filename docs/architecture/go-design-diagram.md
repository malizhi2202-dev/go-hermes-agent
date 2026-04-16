# Go 版本设计图

## 总体设计图

```mermaid
flowchart LR
    subgraph Entry[入口层]
        A1[cmd/hermesd]
        A2[cmd/hermesctl]
    end

    subgraph AppLayer[应用装配层]
        B1[internal/app App]
    end

    subgraph Service[服务层]
        C1[internal/api]
        C2[internal/gateway]
        C3[internal/llm]
    end

    subgraph Security[安全与配置层]
        D1[internal/config]
        D2[internal/auth]
        D3[internal/security]
        D4[internal/execution]
    end

    subgraph Ext[工具与扩展层]
        E1[internal/tools]
        E2[internal/extensions]
    end

    subgraph Data[数据层]
        F1[(SQLite)]
        F2[users]
        F3[sessions/messages]
        F4[messages_fts]
        F5[audit_log]
        F6[extension_states]
    end

    subgraph External[外部依赖]
        G1[OpenAI Compatible API]
        G2[Telegram]
        G3[Plugin Scripts]
        G4[Skill Scripts]
        G5[MCP Servers]
    end

    A1 --> B1
    A2 --> B1

    B1 --> C1
    B1 --> C2
    B1 --> C3
    B1 --> D2
    B1 --> D4
    B1 --> E1
    B1 --> E2
    B1 --> F1

    D1 --> B1
    D3 --> D2
    C3 --> G1
    C2 --> G2

    E2 --> G3
    E2 --> G4
    E2 --> G5

    F1 --> F2
    F1 --> F3
    F1 --> F4
    F1 --> F5
    F1 --> F6
```

## 核心设计原则

### 1. 单一装配中心

所有核心能力都通过 `internal/app.App` 汇总，避免像 Python 版本那样散落在多个运行时注册点。

### 2. 接口入口统一

无论来自：

- HTTP API
- Webhook
- Telegram
- Tool 调用
- Extension 调用

最终都尽量回到统一的 `App` 和 `Store` 边界。

### 3. 高风险能力单独围栏

把高风险执行链单独放在：

- `internal/execution`
- `internal/extensions`

这样可以明确哪些能力需要审批、白名单、审计和后续沙箱。

### 4. 扩展优先“可治理”

Go 版本对动态扩展的设计不是追求最大自由，而是追求：

1. 可发现
2. 可启停
3. 可审计
4. 可限制
5. 可后续替换成更强实现

## 模块关系图

```mermaid
classDiagram
    class App {
        +Config
        +Store
        +Auth
        +LLM
        +Tools
        +Runner
        +Extensions
        +Chat(ctx, username, prompt)
    }

    class Server {
        +Handler()
        +ListenAndServe(ctx)
    }

    class Store {
        +CreateSession()
        +AddMessage()
        +SearchMessages()
        +WriteAudit()
        +ListExtensionStates()
    }

    class Registry {
        +Register()
        +Unregister()
        +Execute()
        +List()
    }

    class Manager {
        +Discover()
        +Register()
        +Summary()
        +SetEnabled()
    }

    class Executor {
        +Execute()
    }

    class Client {
        +Chat()
    }

    App --> Store
    App --> Registry
    App --> Manager
    App --> Executor
    App --> Client
    Server --> App
    Manager --> Registry
```

## 部署设计图

```mermaid
flowchart TD
    A[管理员/客户端] --> B[hermesctl]
    A --> C[hermesd]
    D[Telegram] --> C
    E[Webhook Client] --> C

    C --> F[API Layer]
    F --> G[App Core]
    G --> H[SQLite]
    G --> I[LLM Provider]
    G --> J[Tools]
    G --> K[Extensions]

    K --> L[Plugin Script]
    K --> M[Skill Script]
    K --> N[MCP Server]
```

## 当前设计的优点

- 结构清晰，入口统一
- 安全边界明确
- 状态与审计集中
- 容易继续向更多 Slack 能力、更多 MCP transport、更多 gateway 扩展

## 当前设计的限制

- 还不是完整的多轮 agent orchestration
- MCP 目前已支持 `stdio` 与受控 `http`，但还没有更完整的 streamable HTTP / 长连接 transport
- plugin / skill 目前是受控命令模板，不是完整生命周期插件系统
- 复杂动态执行链仍然保守收口
