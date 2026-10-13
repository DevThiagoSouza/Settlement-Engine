package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/domain/events"
)

var (
	ErrInvalidOutboxEvent = errors.New("invalid outbox event")
	ErrEventNotFound      = errors.New("event not found in outbox")
)

type OutboxEventStatus string

const (
	StatusPending      OutboxEventStatus = "PENDING"
	StatusProcessing   OutboxEventStatus = "PROCESSING"
	StatusPublished    OutboxEventStatus = "PUBLISHED"
	StatusFailed       OutboxEventStatus = "FAILED"
	StatusAcknowledged OutboxEventStatus = "ACKNOWLEDGED"
)

type OutboxEvent struct {
	id             uuid.UUID
	aggregateID    uuid.UUID
	eventType      string
	eventData      []byte
	status         OutboxEventStatus
	publishedAt    *time.Time
	acknowledgedAt *time.Time
	retryCount     int
	maxRetries     int
	createdAt      time.Time
	updatedAt      time.Time
	failureReason  string
}

func NewOutboxEvent(domainEvent events.DomainEvent) (*OutboxEvent, error) {
	if domainEvent == nil {
		return nil, ErrInvalidOutboxEvent
	}

	eventData, err := json.Marshal(domainEvent)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	return &OutboxEvent{
		id:          uuid.New(),
		aggregateID: domainEvent.AggregateID(),
		eventType:   domainEvent.EventType(),
		eventData:   eventData,
		status:      StatusPending,
		retryCount:  0,
		maxRetries:  3,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

func ReconstructOutboxEvent(
	id uuid.UUID,
	aggregateID uuid.UUID,
	eventType string,
	eventData []byte,
	status OutboxEventStatus,
	publishedAt *time.Time,
	acknowledgedAt *time.Time,
	retryCount int,
	maxRetries int,
	createdAt time.Time,
	updatedAt time.Time,
	failureReason string,
) *OutboxEvent {
	return &OutboxEvent{
		id:             id,
		aggregateID:    aggregateID,
		eventType:      eventType,
		eventData:      eventData,
		status:         status,
		publishedAt:    publishedAt,
		acknowledgedAt: acknowledgedAt,
		retryCount:     retryCount,
		maxRetries:     maxRetries,
		createdAt:      createdAt,
		updatedAt:      updatedAt,
		failureReason:  failureReason,
	}
}

func (oe *OutboxEvent) ID() uuid.UUID {
	return oe.id
}

func (oe *OutboxEvent) AggregateID() uuid.UUID {
	return oe.aggregateID
}

func (oe *OutboxEvent) EventType() string {
	return oe.eventType
}

func (oe *OutboxEvent) EventData() []byte {
	return oe.eventData
}

func (oe *OutboxEvent) Status() OutboxEventStatus {
	return oe.status
}

func (oe *OutboxEvent) PublishedAt() *time.Time {
	return oe.publishedAt
}

func (oe *OutboxEvent) AcknowledgedAt() *time.Time {
	return oe.acknowledgedAt
}

func (oe *OutboxEvent) RetryCount() int {
	return oe.retryCount
}

func (oe *OutboxEvent) MaxRetries() int {
	return oe.maxRetries
}

func (oe *OutboxEvent) CreatedAt() time.Time {
	return oe.createdAt
}

func (oe *OutboxEvent) UpdatedAt() time.Time {
	return oe.updatedAt
}

func (oe *OutboxEvent) FailureReason() string {
	return oe.failureReason
}

func (oe *OutboxEvent) MarkAsPublished() {
	oe.status = StatusPublished
	now := time.Now().UTC()
	oe.publishedAt = &now
	oe.updatedAt = now
}

func (oe *OutboxEvent) MarkAsAcknowledged() {
	oe.status = StatusAcknowledged
	now := time.Now().UTC()
	oe.acknowledgedAt = &now
	oe.updatedAt = now
}

func (oe *OutboxEvent) MarkAsFailed(reason string) error {
	oe.retryCount++
	oe.failureReason = reason
	oe.updatedAt = time.Now().UTC()

	if oe.retryCount >= oe.maxRetries {
		oe.status = StatusFailed
		return errors.New("max retries reached")
	}

	oe.status = StatusPending
	return nil
}

func (oe *OutboxEvent) CanRetry() bool {
	return oe.status == StatusPending && oe.retryCount < oe.maxRetries
}

type OutboxRepository interface {
	Save(ctx context.Context, event *OutboxEvent) error

	FindByID(ctx context.Context, id uuid.UUID) (*OutboxEvent, error)

	FindPendingEvents(ctx context.Context, limit int) ([]*OutboxEvent, error)

	ClaimPendingEvents(ctx context.Context, limit int) ([]*OutboxEvent, error)

	FindByAggregateID(ctx context.Context, aggregateID uuid.UUID) ([]*OutboxEvent, error)

	UpdateStatus(ctx context.Context, id uuid.UUID, status OutboxEventStatus) error

	MarkAsAcknowledged(ctx context.Context, id uuid.UUID) error

	MarkAsFailed(ctx context.Context, id uuid.UUID, reason string) error

	GetOldFailedEvents(ctx context.Context, olderThan time.Time, limit int) ([]*OutboxEvent, error)

	DeleteAcknowledgedEvents(ctx context.Context, olderThan time.Time) error
}

type OutboxPublisher interface {
	Publish(ctx context.Context, event *OutboxEvent) error

	PublishBatch(ctx context.Context, events []*OutboxEvent) error
}

type OutboxProcessor struct {
	repository OutboxRepository
	publisher  OutboxPublisher
}

func NewOutboxProcessor(repository OutboxRepository, publisher OutboxPublisher) *OutboxProcessor {
	return &OutboxProcessor{
		repository: repository,
		publisher:  publisher,
	}
}

func (op *OutboxProcessor) ProcessPendingEvents(ctx context.Context, batchSize int) (int, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	pendingEvents, err := op.repository.ClaimPendingEvents(ctx, batchSize)
	if err != nil {
		return 0, err
	}

	if len(pendingEvents) == 0 {
		return 0, nil
	}

	processedCount := 0

	for _, event := range pendingEvents {
		err := op.publisher.Publish(ctx, event)
		if err != nil {
			_ = op.repository.MarkAsFailed(ctx, event.ID(), err.Error())
		} else {
			event.MarkAsPublished()
			if err := op.repository.UpdateStatus(ctx, event.ID(), StatusPublished); err != nil {
				continue
			}
			processedCount++
		}
	}

	return processedCount, nil
}

func (op *OutboxProcessor) AcknowledgeEvent(ctx context.Context, eventID uuid.UUID) error {
	return op.repository.MarkAsAcknowledged(ctx, eventID)
}

func (op *OutboxProcessor) CleanupOldEvents(ctx context.Context, olderThan time.Time) error {
	return op.repository.DeleteAcknowledgedEvents(ctx, olderThan)
}
