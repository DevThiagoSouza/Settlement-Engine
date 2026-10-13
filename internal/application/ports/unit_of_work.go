package ports

import (
	"context"
	"database/sql"
)

type UnitOfWork interface {
	Execute(ctx context.Context, fn func(context.Context) error) error
}

type transactionContextKey struct{}
type principalContextKey struct{}

func WithTransaction(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, transactionContextKey{}, tx)
}

func Transaction(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(transactionContextKey{}).(*sql.Tx)
	return tx, ok
}

func WithPrincipal(ctx context.Context, principal string) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func Principal(ctx context.Context) (string, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(string)
	return principal, ok && principal != ""
}
