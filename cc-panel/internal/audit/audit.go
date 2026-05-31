package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Logger struct {
	db *pgxpool.Pool
}

type Log struct {
	ID          int64          `json:"id"`
	ActorUserID *int64         `json:"actor_user_id,omitempty"`
	Action      string         `json:"action"`
	TargetType  string         `json:"target_type"`
	TargetID    *int64         `json:"target_id,omitempty"`
	Detail      map[string]any `json:"detail"`
	RemoteAddr  string         `json:"remote_addr"`
	CreatedAt   time.Time      `json:"created_at"`
}

type ListResult struct {
	Items    []Log `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func NewLogger(db *pgxpool.Pool) *Logger {
	return &Logger{db: db}
}

func (l *Logger) Record(ctx context.Context, actorUserID *int64, action, targetType string, targetID *int64, detail map[string]any, remoteAddr string) error {
	if detail == nil {
		detail = map[string]any{}
	}
	payload, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal audit detail: %w", err)
	}
	_, err = l.db.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, detail, remote_addr)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, actorUserID, action, targetType, targetID, payload, remoteAddr)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

func (l *Logger) List(ctx context.Context, limit int) ([]Log, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	result, err := l.ListPage(ctx, 1, limit)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (l *Logger) ListPage(ctx context.Context, page, pageSize int) (ListResult, error) {
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	var total int64
	if err := l.db.QueryRow(ctx, `SELECT COUNT(*) FROM audit_logs`).Scan(&total); err != nil {
		return ListResult{}, fmt.Errorf("count audit logs: %w", err)
	}

	rows, err := l.db.Query(ctx, `
		SELECT id, actor_user_id, action, target_type, target_id, detail, remote_addr, created_at
		FROM audit_logs
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return ListResult{}, fmt.Errorf("query audit logs: %w", err)
	}
	defer rows.Close()

	logs := make([]Log, 0)
	for rows.Next() {
		var item Log
		var detail []byte
		if err := rows.Scan(&item.ID, &item.ActorUserID, &item.Action, &item.TargetType, &item.TargetID, &detail, &item.RemoteAddr, &item.CreatedAt); err != nil {
			return ListResult{}, fmt.Errorf("scan audit log: %w", err)
		}
		if err := json.Unmarshal(detail, &item.Detail); err != nil {
			return ListResult{}, fmt.Errorf("unmarshal audit detail: %w", err)
		}
		logs = append(logs, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, err
	}
	return ListResult{
		Items:    logs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
