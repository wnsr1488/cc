# CC Panel — Linux 服务器防 CC 管理面板

面向多台 Linux 服务器的 **防 CC / 防恶意访问** Web 管理面板。通过 SSH 远程统一管理 `iptables` + `ipset` 防护规则，支持地区封禁、自动封禁、系统监控与操作审计。

> 本项目仅用于服务器安全防护与运维管理，不提供攻击或压测能力。

## 功能概览

| 模块 | 说明 |
|------|------|
| 服务器管理 | 资产 CRUD、SSH 连通测试、在线状态、防护模式切换 |
| 黑白名单 | 单 IP / 批量导入，同步写入远程 `cc_whitelist` / `cc_blacklist` |
| 地区封禁 | ip2region 归属查询、CIDR 导入、默认 9 国地区白名单、每日自动同步 |
| 防护模式 | **严格白名单**（非白名单 DROP）与 **连接数封禁**（超阈值写 ipset）二选一 |
| 自动策略 | 基于 `ss` 统计单 IP ESTABLISHED 连接数，超阈值写入 `cc_rate_block` |
| 系统监控 | CPU / 内存 / 负载 / TCP / iptables 命中、实时连接 TOP、封禁 IP 明细 |
| 规则运维 | 部署 / 停止 iptables / 快照回滚、目标服务器批量检测与补挂 |
| 审计日志 | 登录与关键操作记录，支持分页查询 |

## 技术栈

- **后端**：Go 1.22+、Chi、PostgreSQL、JWT
- **前端**：Vue 3、TypeScript、Element Plus、Vite
- **远程执行**：SSH（固定命令模板，禁止任意 shell）
- **地区库**：ip2region xdb
- **防护**：iptables + ipset（后续计划 nftables / Agent 模式）

## 快速开始

### 环境要求

- Go 1.22+
- PostgreSQL 13+
- Node.js 18+（前端开发/构建）
- 被管 Linux 服务器需具备 SSH、`iptables`、`ipset`

### 安装与运行

```bash
cd cc-panel

# 1. 配置环境变量
cp .env.example .env
# 编辑 .env：JWT_SECRET、APP_ENCRYPTION_KEY、DATABASE_URL、ADMIN_PASSWORD 等

# 2. 数据库迁移
go run ./cmd/migrate

# 3. 启动后端（默认 :8080，内置前端静态资源）
go run ./cmd/server
```

前端开发模式（可选）：

```bash
cd cc-panel/web
npm install
npm run dev
```

生产打包：

```bash
cd cc-panel
bash scripts/package.sh   # 生成 linux-amd64 发布包
bash scripts/install.sh   # systemd 安装（可选）
```

默认管理员：`admin` / `.env` 中 `ADMIN_PASSWORD`（首次启动自动创建）。

### 验证

```bash
cd cc-panel
go test ./...
go vet ./...
```

## 项目结构

```
cc/
├── README.md              # 本文件
├── cc.doc                 # 完整设计文档与迭代记录
└── cc-panel/
    ├── cmd/server         # API 服务入口
    ├── cmd/migrate        # 数据库迁移
    ├── internal/          # 后端模块（api、firewall、geo、policy、monitor…）
    ├── migrations/        # SQL 迁移文件
    ├── web/               # Vue 3 前端
    └── scripts/           # 打包与 systemd 安装脚本
```

## 安全说明

- SSH 密码与私钥加密存储，远程命令仅允许预定义模板
- 部署 / 停止规则前自动创建快照，支持回滚
- 白名单规则优先于封禁规则；严格白名单模式需确保管理 IP 已在白名单内
- 所有状态变更操作写入审计日志

## 文档

详细架构、数据表、API 与迭代记录见 [`cc.doc`](./cc.doc)。

子项目说明见 [`cc-panel/README.md`](./cc-panel/README.md)。

## License

Private / 内部使用 — 如需开源协议请自行补充。
