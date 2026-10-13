package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/domain/transaction"
)

var (
	ErrDuplicateTransaction = errors.New("transaction with this idempotency key already exists")
	ErrTransactionNotFound  = transaction.ErrTransactionNotFound
)

type PostgresTransactionRepository struct {
	db *sql.DB
}

func NewPostgresTransactionRepository(db *sql.DB) *PostgresTransactionRepository {
	return &PostgresTransactionRepository{db: db}
}

func (r *PostgresTransactionRepository) Save(ctx context.Context, tx *transaction.Transaction) error {
	query := `
		INSERT INTO transactions 
		(id, idempotency_key, source_account_id, target_account_id, amount, 
		 transaction_type, status, description, version, created_at, processed_at, failure_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (idempotency_key) DO NOTHING
	`

	result, err := executor(ctx, r.db).ExecContext(ctx, query,
		tx.ID(),
		tx.IdempotencyKey(),
		tx.SourceAccountID(),
		tx.TargetAccountID(),
		tx.Amount(),
		string(tx.Type()),
		string(tx.Status()),
		tx.Description(),
		tx.Version(),
		tx.CreatedAt(),
		tx.ProcessedAt(),
		tx.FailureReason(),
	)

	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrDuplicateTransaction
	}

	return nil
}

func (r *PostgresTransactionRepository) SaveWithOptimisticLock(ctx context.Context, tx *transaction.Transaction, expectedVersion int64) error {
	if !tx.CanUpdate(expectedVersion) {
		return ErrConcurrencyConflict
	}

	query := `
		UPDATE transactions 
		SET status = $2, version = $3, processed_at = $4, failure_reason = $5
		WHERE id = $1 AND version = $6
	`

	result, err := executor(ctx, r.db).ExecContext(ctx, query,
		tx.ID(),
		string(tx.Status()),
		tx.Version(),
		tx.ProcessedAt(),
		tx.FailureReason(),
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

func (r *PostgresTransactionRepository) FindByID(ctx context.Context, id uuid.UUID) (*transaction.Transaction, error) {
	query := `
		SELECT id, idempotency_key, source_account_id, target_account_id, amount, 
		       transaction_type, status, description, version, created_at, processed_at, failure_reason
		FROM transactions
		WHERE id = $1
	`

	var (
		txID            uuid.UUID
		idempotencyKey  string
		sourceAccountID uuid.UUID
		targetAccountID uuid.UUID
		amount          int64
		txType          string
		status          string
		description     string
		version         int64
		createdAt       time.Time
		processedAt     *time.Time
		failureReason   string
	)

	err := executor(ctx, r.db).QueryRowContext(ctx, query, id).Scan(
		&txID, &idempotencyKey, &sourceAccountID, &targetAccountID, &amount,
		&txType, &status, &description, &version, &createdAt, &processedAt, &failureReason,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTransactionNotFound
		}
		return nil, err
	}

	return transaction.ReconstructTransaction(
		txID, idempotencyKey, sourceAccountID, targetAccountID, amount,
		transaction.TransactionType(txType), transaction.TransactionStatus(status),
		description, version, createdAt, processedAt, failureReason,
	)
}

func (r *PostgresTransactionRepository) FindByIdempotencyKey(ctx context.Context, key string) (*transaction.Transaction, error) {
	query := `
		SELECT id, idempotency_key, source_account_id, target_account_id, amount, 
		       transaction_type, status, description, version, created_at, processed_at, failure_reason
		FROM transactions
		WHERE idempotency_key = $1
		LIMIT 1
	`

	var (
		txID            uuid.UUID
		idempotencyKey  string
		sourceAccountID uuid.UUID
		targetAccountID uuid.UUID
		amount          int64
		txType          string
		status          string
		description     string
		version         int64
		createdAt       time.Time
		processedAt     *time.Time
		failureReason   string
	)

	err := executor(ctx, r.db).QueryRowContext(ctx, query, key).Scan(
		&txID, &idempotencyKey, &sourceAccountID, &targetAccountID, &amount,
		&txType, &status, &description, &version, &createdAt, &processedAt, &failureReason,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTransactionNotFound
		}
		return nil, err
	}

	return transaction.ReconstructTransaction(
		txID, idempotencyKey, sourceAccountID, targetAccountID, amount,
		transaction.TransactionType(txType), transaction.TransactionStatus(status),
		description, version, createdAt, processedAt, failureReason,
	)
}

func (r *PostgresTransactionRepository) FindBySourceAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*transaction.Transaction, error) {
	query := `
		SELECT id, idempotency_key, source_account_id, target_account_id, amount, 
		       transaction_type, status, description, version, created_at, processed_at, failure_reason
		FROM transactions
		WHERE source_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	return r.queryTransactions(ctx, query, accountID, limit, offset)
}

func (r *PostgresTransactionRepository) FindByTargetAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]*transaction.Transaction, error) {
	query := `
		SELECT id, idempotency_key, source_account_id, target_account_id, amount, 
		       transaction_type, status, description, version, created_at, processed_at, failure_reason
		FROM transactions
		WHERE target_account_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	return r.queryTransactions(ctx, query, accountID, limit, offset)
}

func (r *PostgresTransactionRepository) FindByStatus(ctx context.Context, status transaction.TransactionStatus, limit, offset int) ([]*transaction.Transaction, error) {
	query := `
		SELECT id, idempotency_key, source_account_id, target_account_id, amount, 
		       transaction_type, status, description, version, created_at, processed_at, failure_reason
		FROM transactions
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, string(status), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

func (r *PostgresTransactionRepository) FindAll(ctx context.Context, limit, offset int) ([]*transaction.Transaction, error) {
	query := `
		SELECT id, idempotency_key, source_account_id, target_account_id, amount, 
		       transaction_type, status, description, version, created_at, processed_at, failure_reason
		FROM transactions
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

func (r *PostgresTransactionRepository) GetPendingTransactions(ctx context.Context, limit int) ([]*transaction.Transaction, error) {
	query := `
		SELECT id, idempotency_key, source_account_id, target_account_id, amount, 
		       transaction_type, status, description, version, created_at, processed_at, failure_reason
		FROM transactions
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

func (r *PostgresTransactionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM transactions WHERE id = $1`

	result, err := executor(ctx, r.db).ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrTransactionNotFound
	}

	return nil
}

func (r *PostgresTransactionRepository) queryTransactions(ctx context.Context, query string, args ...interface{}) ([]*transaction.Transaction, error) {
	rows, err := executor(ctx, r.db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTransactions(rows)
}

func (r *PostgresTransactionRepository) scanTransactions(rows *sql.Rows) ([]*transaction.Transaction, error) {
	var transactions []*transaction.Transaction

	for rows.Next() {
		var (
			txID            uuid.UUID
			idempotencyKey  string
			sourceAccountID uuid.UUID
			targetAccountID uuid.UUID
			amount          int64
			txType          string
			status          string
			description     string
			version         int64
			createdAt       time.Time
			processedAt     *time.Time
			failureReason   string
		)

		err := rows.Scan(
			&txID, &idempotencyKey, &sourceAccountID, &targetAccountID, &amount,
			&txType, &status, &description, &version, &createdAt, &processedAt, &failureReason,
		)
		if err != nil {
			return nil, err
		}

		tx, err := transaction.ReconstructTransaction(
			txID, idempotencyKey, sourceAccountID, targetAccountID, amount,
			transaction.TransactionType(txType), transaction.TransactionStatus(status),
			description, version, createdAt, processedAt, failureReason,
		)
		if err != nil {
			return nil, err
		}

		transactions = append(transactions, tx)
	}

	return transactions, rows.Err()
}

type TransactionMetrics struct {
	TotalCount     int64
	ProcessedCount int64
	FailedCount    int64
	PendingCount   int64
	TotalAmount    int64
	AverageAmount  int64
}

func (r *PostgresTransactionRepository) GetMetrics(ctx context.Context) (*TransactionMetrics, error) {
	query := `
		SELECT
			COUNT(*) as total_count,
			COUNT(*) FILTER (WHERE status = 'PROCESSED') as processed_count,
			COUNT(*) FILTER (WHERE status = 'FAILED') as failed_count,
			COUNT(*) FILTER (WHERE status = 'PENDING') as pending_count,
			COALESCE(SUM(amount), 0) as total_amount,
			COALESCE(AVG(amount), 0)::BIGINT as average_amount
		FROM transactions
	`

	var metrics TransactionMetrics
	err := executor(ctx, r.db).QueryRowContext(ctx, query).Scan(
		&metrics.TotalCount,
		&metrics.ProcessedCount,
		&metrics.FailedCount,
		&metrics.PendingCount,
		&metrics.TotalAmount,
		&metrics.AverageAmount,
	)

	if err != nil {
		return nil, err
	}

	return &metrics, nil
}
