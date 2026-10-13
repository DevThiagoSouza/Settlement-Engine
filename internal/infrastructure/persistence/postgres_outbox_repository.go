package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/infrastructure/outbox"
)

var (
	ErrOutboxEventNotFound = errors.New("outbox event not found")
)

type PostgresOutboxRepository struct {
	db *sql.DB
}

func NewPostgresOutboxRepository(db *sql.DB) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{db: db}
}

func (r *PostgresOutboxRepository) Save(ctx context.Context, event *outbox.OutboxEvent) error {
	query := `
		INSERT INTO outbox_events 
		(id, aggregate_id, event_type, event_data, status, retry_count, max_retries, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`

	_, err := executor(ctx, r.db).ExecContext(ctx, query,
		event.ID(),
		event.AggregateID(),
		event.EventType(),
		event.EventData(),
		string(event.Status()),
		event.RetryCount(),
		event.MaxRetries(),
		event.CreatedAt(),
		event.UpdatedAt(),
	)

	return err
}

func (r *PostgresOutboxRepository) FindByID(ctx context.Context, id uuid.UUID) (*outbox.OutboxEvent, error) {
	query := `
		SELECT id, aggregate_id, event_type, event_data, status, published_at, acknowledged_at, 
		       retry_count, max_retries, created_at, updated_at, failure_reason
		FROM outbox_events
		WHERE id = $1
	`

	var (
		eventID        uuid.UUID
		aggregateID    uuid.UUID
		eventType      string
		eventData      []byte
		status         string
		publishedAt    *time.Time
		acknowledgedAt *time.Time
		retryCount     int
		maxRetries     int
		createdAt      time.Time
		updatedAt      time.Time
		failureReason  string
	)

	err := executor(ctx, r.db).QueryRowContext(ctx, query, id).Scan(
		&eventID, &aggregateID, &eventType, &eventData, &status, &publishedAt, &acknowledgedAt,
		&retryCount, &maxRetries, &createdAt, &updatedAt, &failureReason,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrOutboxEventNotFound
		}
		return nil, err
	}

	return outbox.ReconstructOutboxEvent(
		eventID, aggregateID, eventType, eventData,
		outbox.OutboxEventStatus(status),
		publishedAt, acknowledgedAt, retryCount, maxRetries,
		createdAt, updatedAt, failureReason,
	), nil
}

func (r *PostgresOutboxRepository) FindPendingEvents(ctx context.Context, limit int) ([]*outbox.OutboxEvent, error) {
	query := `
		SELECT id, aggregate_id, event_type, event_data, status, published_at, acknowledged_at, 
		       retry_count, max_retries, created_at, updated_at, failure_reason
		FROM outbox_events
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanOutboxEvents(rows)
}

func (r *PostgresOutboxRepository) ClaimPendingEvents(ctx context.Context, limit int) ([]*outbox.OutboxEvent, error) {
	query := `
		WITH claimed AS (
			SELECT id
			FROM outbox_events
			WHERE status = 'PENDING'
			   OR (status = 'PROCESSING' AND updated_at < CURRENT_TIMESTAMP - INTERVAL '5 minutes')
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events AS event
		SET status = 'PROCESSING', updated_at = $2
		FROM claimed
		WHERE event.id = claimed.id
		RETURNING event.id, event.aggregate_id, event.event_type, event.event_data,
		          event.status, event.published_at, event.acknowledged_at,
		          event.retry_count, event.max_retries, event.created_at,
		          event.updated_at, event.failure_reason
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, limit, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.scanOutboxEvents(rows)
}

func (r *PostgresOutboxRepository) FindByAggregateID(ctx context.Context, aggregateID uuid.UUID) ([]*outbox.OutboxEvent, error) {
	query := `
		SELECT id, aggregate_id, event_type, event_data, status, published_at, acknowledged_at, 
		       retry_count, max_retries, created_at, updated_at, failure_reason
		FROM outbox_events
		WHERE aggregate_id = $1
		ORDER BY created_at DESC
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, aggregateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanOutboxEvents(rows)
}

func (r *PostgresOutboxRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status outbox.OutboxEventStatus) error {
	query := `
		UPDATE outbox_events
		SET status = $2, updated_at = $3
		WHERE id = $1
	`

	result, err := executor(ctx, r.db).ExecContext(ctx, query, id, string(status), time.Now().UTC())
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrOutboxEventNotFound
	}

	return nil
}

func (r *PostgresOutboxRepository) MarkAsAcknowledged(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE outbox_events
		SET status = 'ACKNOWLEDGED', acknowledged_at = $2, updated_at = $2
		WHERE id = $1
	`

	now := time.Now().UTC()
	result, err := executor(ctx, r.db).ExecContext(ctx, query, id, now)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrOutboxEventNotFound
	}

	return nil
}

func (r *PostgresOutboxRepository) MarkAsFailed(ctx context.Context, id uuid.UUID, reason string) error {
	query := `
		UPDATE outbox_events
		SET retry_count = retry_count + 1,
		    status = CASE 
		              WHEN retry_count + 1 >= max_retries THEN 'FAILED'
		              ELSE 'PENDING'
		            END,
		    failure_reason = $3,
		    updated_at = $2
		WHERE id = $1
	`

	now := time.Now().UTC()
	result, err := executor(ctx, r.db).ExecContext(ctx, query, id, now, reason)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrOutboxEventNotFound
	}

	return nil
}

func (r *PostgresOutboxRepository) GetOldFailedEvents(ctx context.Context, olderThan time.Time, limit int) ([]*outbox.OutboxEvent, error) {
	query := `
		SELECT id, aggregate_id, event_type, event_data, status, published_at, acknowledged_at, 
		       retry_count, max_retries, created_at, updated_at, failure_reason
		FROM outbox_events
		WHERE status = 'FAILED' AND updated_at < $1
		ORDER BY updated_at ASC
		LIMIT $2
	`

	rows, err := executor(ctx, r.db).QueryContext(ctx, query, olderThan, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanOutboxEvents(rows)
}

func (r *PostgresOutboxRepository) DeleteAcknowledgedEvents(ctx context.Context, olderThan time.Time) error {
	query := `
		DELETE FROM outbox_events
		WHERE status = 'ACKNOWLEDGED' AND acknowledged_at < $1
	`

	_, err := executor(ctx, r.db).ExecContext(ctx, query, olderThan)
	return err
}

func (r *PostgresOutboxRepository) scanOutboxEvents(rows *sql.Rows) ([]*outbox.OutboxEvent, error) {
	var events []*outbox.OutboxEvent

	for rows.Next() {
		var (
			eventID        uuid.UUID
			aggregateID    uuid.UUID
			eventType      string
			eventData      []byte
			status         string
			publishedAt    *time.Time
			acknowledgedAt *time.Time
			retryCount     int
			maxRetries     int
			createdAt      time.Time
			updatedAt      time.Time
			failureReason  string
		)

		err := rows.Scan(
			&eventID, &aggregateID, &eventType, &eventData, &status, &publishedAt, &acknowledgedAt,
			&retryCount, &maxRetries, &createdAt, &updatedAt, &failureReason,
		)
		if err != nil {
			return nil, err
		}

		event := outbox.ReconstructOutboxEvent(
			eventID, aggregateID, eventType, eventData,
			outbox.OutboxEventStatus(status),
			publishedAt, acknowledgedAt, retryCount, maxRetries,
			createdAt, updatedAt, failureReason,
		)

		events = append(events, event)
	}

	return events, rows.Err()
}

type OutboxStatistics struct {
	TotalEvents        int64
	PendingEvents      int64
	PublishedEvents    int64
	FailedEvents       int64
	AcknowledgedEvents int64
}

func (r *PostgresOutboxRepository) GetStatistics(ctx context.Context) (*OutboxStatistics, error) {
	query := `
		SELECT
			COUNT(*) as total_events,
			COUNT(*) FILTER (WHERE status = 'PENDING') as pending_events,
			COUNT(*) FILTER (WHERE status = 'PUBLISHED') as published_events,
			COUNT(*) FILTER (WHERE status = 'FAILED') as failed_events,
			COUNT(*) FILTER (WHERE status = 'ACKNOWLEDGED') as acknowledged_events
		FROM outbox_events
	`

	var stats OutboxStatistics
	err := executor(ctx, r.db).QueryRowContext(ctx, query).Scan(
		&stats.TotalEvents,
		&stats.PendingEvents,
		&stats.PublishedEvents,
		&stats.FailedEvents,
		&stats.AcknowledgedEvents,
	)

	if err != nil {
		return nil, err
	}

	return &stats, nil
}
