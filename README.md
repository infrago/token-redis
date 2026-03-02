# token-redis

`token-redis` 是 `token` 模块的 Redis `Driver` 实现（driver: `github.com/infrago/token-redis`）。

## 包定位

- 类型：驱动（Driver）
- 作用：生产场景共享存储，实现 payload + revoke 的统一持久化
- 特点：一个 redis client 同时处理 payload 与 revoke

## 主要功能

- 生命周期：`Open/Close`（连接池管理）
- `payload` 存储：`SavePayload/LoadPayload/DeletePayload`
- `revoke` 存储：`RevokeToken/RevokeTokenID/Revoked*`
- 键前缀隔离，token revoke key 使用哈希

## 配置（token.setting）

```toml
[token.setting]
redis_addr = "127.0.0.1:6379"
redis_username = ""
redis_password = ""
redis_db = 0
redis_prefix = "infrago:token:"
```

兼容简写键：`addr/username/password/db/prefix`。

## 使用

```go
import _ "github.com/infrago/token-redis"
```

```toml
[token]
driver = "redis"
payload = "token" # token | store | hybrid
```
