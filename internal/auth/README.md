# internal/auth

这个目录负责 Go 版本的本地账号认证。

当前能力：

- 初始化管理员账号
- 本地密码校验
- 登录失败限制
- JWT 签发与解析

设计原则：

- 认证逻辑和持久化分离
- 密码和 JWT 具体实现下沉到 `internal/security`
