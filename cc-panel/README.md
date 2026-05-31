# CC Panel

Go + Vue 3 实现的 Linux 服务器防 CC 管理面板（SSH 无 Agent 模式）。

## 已实现能力

- JWT 登录、操作审计（分页）
- 服务器资产管理与 SSH 测试
- 远程 `ipset` / `iptables` 初始化、增量部署、停止规则、快照回滚
- 黑白名单（含批量添加、按服务器 Tab 分页）
- 地区 CIDR 管理、ip2region 查询、默认 9 国白名单、每日自动同步
- 防护模式：`strict_whitelist` / `connection_count` / `off`
- 自动封禁策略（`connection_count` + `ss` 采集，策略执行事件分页）
- 系统监控采集、实时连接/封禁洞察
- 远程自动安装 `ipset` / `iptables` 依赖

## 环境变量

见 [`.env.example`](./.env.example)：

| 变量 | 说明 |
|------|------|
| `HTTP_ADDR` | 监听地址，默认 `:8080` |
| `DATABASE_URL` | PostgreSQL 连接串 |
| `JWT_SECRET` | JWT 密钥（≥32 字符） |
| `APP_ENCRYPTION_KEY` | 凭据加密密钥（32 字节） |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | 初始管理员 |
| `IP2REGION_V4_XDB` / `IP2REGION_V6_XDB` | ip2region 数据库路径 |

## 常用命令

```bash
# 迁移
go run ./cmd/migrate

# 开发启动
go run ./cmd/server

# 测试
go test ./...

# 前端构建
cd web && npm install && npm run build

# 打包发布
bash scripts/package.sh
```

## API 示例

```bash
# 登录
curl -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me"}'

# 部署防护规则
curl -X POST http://127.0.0.1:8080/api/v1/servers/1/deploy \
  -H "Authorization: Bearer $TOKEN"

# 停止 iptables 规则（保留 ipset）
curl -X POST http://127.0.0.1:8080/api/v1/servers/1/stop-rules \
  -H "Authorization: Bearer $TOKEN"
```

## 目录说明

```text
cmd/server          API 服务（含静态前端）
cmd/migrate         数据库迁移
internal/api        HTTP 路由
internal/firewall   防火墙编排（部署/停止/回滚/黑白名单）
internal/geo        地区 CIDR 与默认白名单
internal/policy     自动封禁策略
internal/monitor    监控采集与实时洞察
internal/iptables   iptables 脚本模板
internal/ipset      ipset 脚本模板
migrations/         SQL  schema
web/                Vue 3 前端
scripts/            打包与安装
```

完整设计文档见仓库根目录 [`../cc.doc`](../cc.doc)。
