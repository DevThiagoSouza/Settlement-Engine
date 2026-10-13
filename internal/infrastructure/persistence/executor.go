package persistence

import (
	"context"
	"database/sql"

	"github.com/settlement-engine/internal/application/ports"
)

type queryExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func executor(ctx context.Context, db *sql.DB) queryExecutor {
	if tx, ok := ports.Transaction(ctx); ok {
		return tx
	}
	return db
}
