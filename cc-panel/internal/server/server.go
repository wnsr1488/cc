package server

import (
	"context"
	"fmt"
	"time"

	secretcrypto "github.com/example/cc-panel/internal/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	AuthTypePassword   = "password"
	AuthTypePrivateKey = "private_key"

	WhitelistModeOff             = "off"
	WhitelistModeStrict          = "strict_whitelist"
	WhitelistModeConnectionCount = "connection_count"
)

func ValidWhitelistMode(mode string) bool {
	switch mode {
	case WhitelistModeOff, WhitelistModeStrict, WhitelistModeConnectionCount:
		return true
	default:
		return false
	}
}

func UsesStrictWhitelist(mode string) bool {
	return mode == WhitelistModeStrict
}

type Server struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Host           string     `json:"host"`
	Port           int        `json:"port"`
	Username       string     `json:"username"`
	AuthType       string     `json:"auth_type"`
	PasswordEnc    string     `json:"-"`
	PrivateKeyEnc  string     `json:"-"`
	GroupName      *string    `json:"group_name,omitempty"`
	OSInfo         *string    `json:"os_info,omitempty"`
	KernelVersion  *string    `json:"kernel_version,omitempty"`
	Status          string     `json:"status"`
	WhitelistMode   string     `json:"whitelist_mode"`
	StrictWhitelist bool       `json:"strict_whitelist"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CreateInput struct {
	Name       string  `json:"name"`
	Host       string  `json:"host"`
	Port       int     `json:"port"`
	Username   string  `json:"username"`
	AuthType   string  `json:"auth_type"`
	Password   string  `json:"password,omitempty"`
	PrivateKey string  `json:"private_key,omitempty"`
	GroupName  *string `json:"group_name,omitempty"`
}

type UpdateInput struct {
	Name            string  `json:"name"`
	Host            string  `json:"host"`
	Port            int     `json:"port"`
	Username        string  `json:"username"`
	AuthType        string  `json:"auth_type"`
	Password        *string `json:"password,omitempty"`
	PrivateKey      *string `json:"private_key,omitempty"`
	GroupName       *string `json:"group_name,omitempty"`
	WhitelistMode   *string `json:"whitelist_mode,omitempty"`
	StrictWhitelist *bool   `json:"strict_whitelist,omitempty"`
}

type Repository struct {
	db  *pgxpool.Pool
	box *secretcrypto.SecretBox
}

func NewRepository(db *pgxpool.Pool, box *secretcrypto.SecretBox) *Repository {
	return &Repository{db: db, box: box}
}

func (r *Repository) Create(ctx context.Context, input CreateInput) (Server, error) {
	if input.Port == 0 {
		input.Port = 22
	}
	if err := input.Validate(); err != nil {
		return Server{}, err
	}
	passwordEnc, privateKeyEnc, err := r.encryptSecrets(input.AuthType, input.Password, input.PrivateKey)
	if err != nil {
		return Server{}, err
	}
	var item Server
	err = r.db.QueryRow(ctx, `
		INSERT INTO servers (name, host, port, username, auth_type, password_enc, private_key_enc, group_name)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, host, port, username, auth_type, password_enc, private_key_enc, group_name, os_info,
			kernel_version, status, whitelist_mode, strict_whitelist, last_seen_at, created_at, updated_at
	`, input.Name, input.Host, input.Port, input.Username, input.AuthType, passwordEnc, privateKeyEnc, input.GroupName).Scan(
		&item.ID, &item.Name, &item.Host, &item.Port, &item.Username, &item.AuthType, &item.PasswordEnc,
		&item.PrivateKeyEnc, &item.GroupName, &item.OSInfo, &item.KernelVersion, &item.Status, &item.WhitelistMode,
		&item.StrictWhitelist, &item.LastSeenAt, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) List(ctx context.Context) ([]Server, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, host, port, username, auth_type, password_enc, private_key_enc, group_name, os_info,
			kernel_version, status, whitelist_mode, strict_whitelist, last_seen_at, created_at, updated_at
		FROM servers
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Server, 0)
	for rows.Next() {
		item, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id int64) (Server, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, name, host, port, username, auth_type, password_enc, private_key_enc, group_name, os_info,
			kernel_version, status, whitelist_mode, strict_whitelist, last_seen_at, created_at, updated_at
		FROM servers
		WHERE id = $1
	`, id)
	return scanServer(row)
}

func (r *Repository) Update(ctx context.Context, id int64, input UpdateInput) (Server, error) {
	if err := input.Validate(); err != nil {
		return Server{}, err
	}
	current, err := r.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	passwordEnc := current.PasswordEnc
	privateKeyEnc := current.PrivateKeyEnc
	if input.Password != nil || input.PrivateKey != nil || input.AuthType != current.AuthType {
		password := ""
		privateKey := ""
		if input.Password != nil {
			password = *input.Password
		}
		if input.PrivateKey != nil {
			privateKey = *input.PrivateKey
		}
		passwordEnc, privateKeyEnc, err = r.encryptSecrets(input.AuthType, password, privateKey)
		if err != nil {
			return Server{}, err
		}
	}

	strictWhitelist := current.StrictWhitelist
	whitelistMode := current.WhitelistMode
	if input.WhitelistMode != nil {
		if !ValidWhitelistMode(*input.WhitelistMode) {
			return Server{}, fmt.Errorf("invalid whitelist_mode %q", *input.WhitelistMode)
		}
		whitelistMode = *input.WhitelistMode
		strictWhitelist = UsesStrictWhitelist(whitelistMode)
	} else if input.StrictWhitelist != nil {
		strictWhitelist = *input.StrictWhitelist
		if strictWhitelist {
			whitelistMode = WhitelistModeStrict
		} else if whitelistMode == WhitelistModeStrict {
			whitelistMode = WhitelistModeOff
		}
	}

	var item Server
	err = r.db.QueryRow(ctx, `
		UPDATE servers
		SET name = $1, host = $2, port = $3, username = $4, auth_type = $5, password_enc = $6,
			private_key_enc = $7, group_name = $8, whitelist_mode = $9, strict_whitelist = $10, updated_at = NOW()
		WHERE id = $11
		RETURNING id, name, host, port, username, auth_type, password_enc, private_key_enc, group_name, os_info,
			kernel_version, status, whitelist_mode, strict_whitelist, last_seen_at, created_at, updated_at
	`, input.Name, input.Host, input.Port, input.Username, input.AuthType, passwordEnc, privateKeyEnc, input.GroupName, whitelistMode, strictWhitelist, id).Scan(
		&item.ID, &item.Name, &item.Host, &item.Port, &item.Username, &item.AuthType, &item.PasswordEnc,
		&item.PrivateKeyEnc, &item.GroupName, &item.OSInfo, &item.KernelVersion, &item.Status, &item.WhitelistMode,
		&item.StrictWhitelist, &item.LastSeenAt, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, "DELETE FROM servers WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) SetWhitelistMode(ctx context.Context, id int64, mode string) error {
	if !ValidWhitelistMode(mode) {
		return fmt.Errorf("invalid whitelist_mode %q", mode)
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE servers
		SET whitelist_mode = $1, strict_whitelist = $2, updated_at = NOW()
		WHERE id = $3
	`, mode, UsesStrictWhitelist(mode), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) SetStrictWhitelist(ctx context.Context, id int64, enabled bool) error {
	mode := WhitelistModeOff
	if enabled {
		mode = WhitelistModeStrict
	}
	return r.SetWhitelistMode(ctx, id, mode)
}

func (r *Repository) MarkOnline(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, "UPDATE servers SET status = 'online', last_seen_at = NOW(), updated_at = NOW() WHERE id = $1", id)
	return err
}

func (r *Repository) MarkOffline(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, "UPDATE servers SET status = 'offline', updated_at = NOW() WHERE id = $1", id)
	return err
}

func (r *Repository) Credentials(item Server) (password, privateKey string, err error) {
	password, err = r.box.Decrypt(item.PasswordEnc)
	if err != nil {
		return "", "", err
	}
	privateKey, err = r.box.Decrypt(item.PrivateKeyEnc)
	if err != nil {
		return "", "", err
	}
	return password, privateKey, nil
}

func (r *Repository) encryptSecrets(authType, password, privateKey string) (string, string, error) {
	switch authType {
	case AuthTypePassword:
		encrypted, err := r.box.Encrypt(password)
		return encrypted, "", err
	case AuthTypePrivateKey:
		encrypted, err := r.box.Encrypt(privateKey)
		return "", encrypted, err
	default:
		return "", "", fmt.Errorf("unsupported auth_type %q", authType)
	}
}

type scanner interface {
	Scan(dest ...any) error
}

func scanServer(row scanner) (Server, error) {
	var item Server
	err := row.Scan(
		&item.ID, &item.Name, &item.Host, &item.Port, &item.Username, &item.AuthType, &item.PasswordEnc,
		&item.PrivateKeyEnc, &item.GroupName, &item.OSInfo, &item.KernelVersion, &item.Status, &item.WhitelistMode,
		&item.StrictWhitelist, &item.LastSeenAt, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}
