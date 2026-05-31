# CC Panel Backend

Go backend MVP for a Linux server anti-CC management panel.

## Features

- PostgreSQL storage
- Initial admin bootstrap
- JWT authentication
- Server asset CRUD
- SSH connection test
- Remote ipset and iptables initialization
- Blacklist and whitelist synchronization through ipset
- Audit logs for state-changing actions

## Requirements

- Go 1.22+
- PostgreSQL 13+
- Managed Linux servers with `ipset`, `iptables`, and SSH access

## Setup

1. Configure the service.

```bash
cp .env.example .env
```

Set secure values for:

- `JWT_SECRET`: at least 32 characters
- `APP_ENCRYPTION_KEY`: exactly 32 characters
- `ADMIN_PASSWORD`: initial administrator password

2. Run database migrations.

```bash
go run ./cmd/migrate
```

3. Start the API.

```bash
go run ./cmd/server
```

### Validation

```bash
go test ./...
go vet ./...
```

## API

Login:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me"}'
```

Create a server:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/servers \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "web-01",
    "host": "192.0.2.10",
    "port": 22,
    "username": "root",
    "auth_type": "password",
    "password": "server-password"
  }'
```

Deploy ipset and iptables base rules:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/servers/1/deploy \
  -H "Authorization: Bearer $TOKEN"
```

Add a blacklist IP:

```bash
curl -X POST http://127.0.0.1:8080/api/v1/firewall/blacklist \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"server_ids":[1],"ip":"8.8.8.8","timeout":0,"reason":"manual block"}'
```

## Security Notes

- The API never accepts arbitrary shell commands from users.
- Remote firewall operations are generated from fixed command templates.
- SSH passwords and private keys are encrypted before storage.
- Whitelist rules are inserted before deny rules.
- Audit logs are written for authentication and state-changing API calls.

## Project Layout

```text
cmd/server      API server entrypoint
cmd/migrate     PostgreSQL migration runner
internal/api    HTTP routing and handlers
internal/auth   JWT, password hashing, and auth middleware
internal/server Server asset repository and validation
internal/sshx   SSH executor
internal/ipset  Safe ipset command templates
internal/iptables Safe iptables deployment templates
internal/firewall Firewall orchestration service
migrations      SQL schema migrations
scripts         Local helper scripts
```
