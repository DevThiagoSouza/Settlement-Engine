package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/settlement-engine/internal/domain/account"
)

var (
	ErrConcurrencyConflict = errors.New("concurrency conflict: account was modified")
	ErrNotFound            = errors.New("account not found")
)

type PostgresAccountRepository struct {
	db *sql.DB
}

func NewPostgresAccountRepository(db *sql.DB) *PostgresAccountRepository {
	return &PostgresAccountRepository{db: db}
}

func (r *PostgresAccountRepository) Save(ctx context.Context, acc *account.Account) error {
	query := `
		INSERT INTO accounts (id, customer_id, balance, version, created_at, updated_at, last_modified)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			balance = EXCLUDED.balance,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			last_modified = EXCLUDED.last_modified
		WHERE accounts.version = $4 - 1
	`

	result, err := executor(ctx, r.db).ExecContext(ctx, query,
		acc.ID(),
		acc.CustomerID(),
		acc.Balance(),
		acc.Version(),
		acc.CreatedAt(),
		acc.UpdatedAt(),
		acc.LastModified(),
	)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		var version int64
		checkErr := executor(ctx, r.db).QueryRowContext(ctx,
			"SELECT version FROM accounts WHERE id = $1",
			acc.ID(),
		).Scan(&version)

		if checkErr == nil && version != acc.Version()-1 {
			return ErrConcurrencyConflict
		}
	}

	return nil
}

func (r *PostgresAccountRepository) SaveWithOptimisticLock(ctx context.Context, acc *account.Account, expectedVersion int64) error {
	if !acc.CanUpdate(expectedVersion) {
		return ErrConcurrencyConflict
	}

	query := `
		UPDATE accounts 
		SET balance = $2, version = $3, updated_at = $4, last_modified = $5
		WHERE id = $1 AND version = $6
	`

	result, err := executor(ctx, r.db).ExecContext(ctx, query,
		acc.ID(),
		acc.Balance(),
		acc.Version(),
		acc.UpdatedAt(),
		acc.LastModified(),
		expectedVersion,
	)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrConcurrencyConflict
	}

	return nil
}

func (r *PostgresAccountRepository) FindByID(ctx context.Context, id uuid.UUID) (*account.Account, error) {
	query := `
		SELECT id, customer_id, balance, version, created_at, updated_at, last_modified
		FROM accounts
		WHERE id = $1
	`

	var (
		accID        uuid.UUID
		customerID   string
		balance      int64
		version      int64
		createdAt    time.Time
		updatedAt    time.Time
		lastModified time.Time
	)

	err := executor(ctx, r.db).QueryRowContext(ctx, query, id).Scan(
		&accID, &customerID, &balance, &version, &createdAt, &updatedAt, &lastModified,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return account.ReconstructAccount(accID, customerID, balance, version, createdAt, updatedAt, lastModified)
}

func (r *PostgresAccountRepository) FindByCustomerID(ctx context.Context, customerID string) (*account.Account, error) {
	query := `
		SELECT id, customer_id, balance, version, created_at, updated_at, last_modified
		FROM accounts
		WHERE customer_id = $1
		LIMIT 1
	`

	var (
		accID        uuid.UUID
		custID       string
		balance      int64
		version      int64
		createdAt    time.Time
		updatedAt    time.Time
		lastModified time.Time
	)

	err := executor(ctx, r.db).QueryRowContext(ctx, query, customerID).Scan(
		&accID, &custID, &balance, &version, &createdAt, &updatedAt, &lastModified,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return account.ReconstructAccount(accID, custID, balance, version, createdAt, updatedAt, lastModified)
}

func (r *PostgresAccountRepository) FindAll(ctx context.Context, limit, offset int) ([]*account.Account, error) {
	query := `
		SELECT id, customer_id, balance, version, created_at, updated_at, last_modified
		FROM accounts
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*account.Account

	for rows.Next() {
		var (
			accID        uuid.UUID
			customerID   string
			balance      int64
			version      int64
			createdAt    time.Time
			updatedAt    time.Time
			lastModified time.Time
		)

		err := rows.Scan(
			&accID, &customerID, &balance, &version, &createdAt, &updatedAt, &lastModified,
		)
		if err != nil {
			return nil, err
		}

		acc, err := account.ReconstructAccount(accID, customerID, balance, version, createdAt, updatedAt, lastModified)
		if err != nil {
			return nil, err
		}

		accounts = append(accounts, acc)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return accounts, nil
}

func (r *PostgresAccountRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM accounts WHERE id = $1`

	result, err := executor(ctx, r.db).ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *PostgresAccountRepository) GetBalance(ctx context.Context, id uuid.UUID) (int64, error) {
	query := `SELECT balance FROM accounts WHERE id = $1`

	var balance int64
	err := executor(ctx, r.db).QueryRowContext(ctx, query, id).Scan(&balance)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrNotFound
		}
		return 0, err
	}

	return balance, nil
}

func (r *PostgresAccountRepository) GetAccountsByIDs(ctx context.Context, ids []uuid.UUID) ([]*account.Account, error) {
	if len(ids) == 0 {
		return []*account.Account{}, nil
	}

	query := `
		SELECT id, customer_id, balance, version, created_at, updated_at, last_modified
		FROM accounts
		WHERE id = ANY($1)
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*account.Account

	for rows.Next() {
		var (
			accID        uuid.UUID
			customerID   string
			balance      int64
			version      int64
			createdAt    time.Time
			updatedAt    time.Time
			lastModified time.Time
		)

		err := rows.Scan(
			&accID, &customerID, &balance, &version, &createdAt, &updatedAt, &lastModified,
		)
		if err != nil {
			return nil, err
		}

		acc, err := account.ReconstructAccount(accID, customerID, balance, version, createdAt, updatedAt, lastModified)
		if err != nil {
			return nil, err
		}

		accounts = append(accounts, acc)
	}

	return accounts, rows.Err()
}

func (r *PostgresAccountRepository) GetBalanceForUpdate(ctx context.Context, id uuid.UUID) (int64, int64, error) {
	query := `
		SELECT balance, version FROM accounts 
		WHERE id = $1 
		FOR UPDATE
	`

	var balance, version int64
	err := executor(ctx, r.db).QueryRowContext(ctx, query, id).Scan(&balance, &version)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, ErrNotFound
		}
		return 0, 0, err
	}

	return balance, version, nil
}

func (r *PostgresAccountRepository) GetDailyBalanceSnapshots(ctx context.Context, accountID uuid.UUID, date time.Time) ([]*account.Account, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
		SELECT id, customer_id, balance, version, created_at, updated_at, last_modified
		FROM accounts
		WHERE id = $1 AND last_modified >= $2 AND last_modified < $3
		ORDER BY last_modified DESC
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, accountID, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*account.Account

	for rows.Next() {
		var (
			accID        uuid.UUID
			customerID   string
			balance      int64
			version      int64
			createdAt    time.Time
			updatedAt    time.Time
			lastModified time.Time
		)

		err := rows.Scan(
			&accID, &customerID, &balance, &version, &createdAt, &updatedAt, &lastModified,
		)
		if err != nil {
			return nil, err
		}

		acc, err := account.ReconstructAccount(accID, customerID, balance, version, createdAt, updatedAt, lastModified)
		if err != nil {
			return nil, err
		}

		accounts = append(accounts, acc)
	}

	return accounts, rows.Err()
}
