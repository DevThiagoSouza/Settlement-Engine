package database

import (
	"context"
	"database/sql"

	"github.com/settlement-engine/internal/application/ports"
)

type PostgresUnitOfWork struct {
	db *sql.DB
}

func NewPostgresUnitOfWork(db *sql.DB) *PostgresUnitOfWork {
	return &PostgresUnitOfWork{db: db}
}

func (u *PostgresUnitOfWork) Execute(ctx context.Context, fn func(context.Context) error) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	txCtx := ports.WithTransaction(ctx, tx)
	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
