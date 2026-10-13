package kafka

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"strings"
	"sync"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/settlement-engine/internal/infrastructure/logging"
	"github.com/settlement-engine/internal/infrastructure/outbox"
)

type KafkaPublisher struct {
	brokers  string
	topic    string
	logger   logging.Logger
	mockMode bool
	writer   *kafka.Writer
	mu       sync.Mutex
	events   []interface{}
}

func NewKafkaPublisher(brokers, topic string, logger logging.Logger) *KafkaPublisher {
	return NewKafkaPublisherWithConnection(brokers, topic, "", logger)
}

func NewKafkaPublisherWithConnection(brokers, topic, connectionString string, logger logging.Logger) *KafkaPublisher {
	brokerList := strings.Split(brokers, ",")
	writerConfig := kafka.WriterConfig{
		Brokers:  brokerList,
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	}
	if connectionString != "" {
		writerConfig.Dialer = &kafka.Dialer{
			TLS: &tls.Config{MinVersion: tls.VersionTLS12},
			SASLMechanism: plain.Mechanism{
				Username: "$ConnectionString",
				Password: connectionString,
			},
		}
	}

	return &KafkaPublisher{
		brokers:  brokers,
		topic:    topic,
		logger:   logger,
		mockMode: false,
		writer:   kafka.NewWriter(writerConfig),
		events:   make([]interface{}, 0),
	}
}

func (kp *KafkaPublisher) EnableMockMode() {
	kp.mockMode = true
}

func (kp *KafkaPublisher) Publish(ctx context.Context, event *outbox.OutboxEvent) error {
	if kp.mockMode {
		return kp.publishMock(ctx, event)
	}

	kp.logger.Info(ctx, "Publishing event to Kafka",
		"event_id", event.ID().String(),
		"event_type", event.EventType(),
		"topic", kp.topic,
	)

	if err := kp.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(event.ID().String()),
		Value: event.EventData(),
	}); err != nil {
		return err
	}

	event.MarkAsPublished()
	kp.logger.Info(ctx, "Event published successfully", event.ID().String())
	return nil
}

func (kp *KafkaPublisher) PublishBatch(ctx context.Context, events []*outbox.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}

	kp.logger.Info(ctx, "Publishing batch of events",
		"count", len(events),
		"topic", kp.topic,
	)

	for _, event := range events {
		if err := kp.Publish(ctx, event); err != nil {
			kp.logger.Error(ctx, "Failed to publish event in batch", err.Error())
			return err
		}
	}

	kp.logger.Info(ctx, "Batch published successfully", len(events))
	return nil
}

func (kp *KafkaPublisher) publishMock(ctx context.Context, event *outbox.OutboxEvent) error {
	kp.mu.Lock()
	defer kp.mu.Unlock()

	var eventData interface{}
	if err := json.Unmarshal(event.EventData(), &eventData); err != nil {
		return err
	}

	kp.events = append(kp.events, eventData)
	kp.logger.Debug(ctx, "Event stored in mock buffer", event.EventType())
	return nil
}

func (kp *KafkaPublisher) GetMockEvents() []interface{} {
	kp.mu.Lock()
	defer kp.mu.Unlock()

	result := make([]interface{}, len(kp.events))
	copy(result, kp.events)
	return result
}

func (kp *KafkaPublisher) ClearMockEvents() {
	kp.mu.Lock()
	defer kp.mu.Unlock()
	kp.events = make([]interface{}, 0)
}

type MockPublisher struct {
	mu     sync.Mutex
	events []*outbox.OutboxEvent
	logger logging.Logger
}

func NewMockPublisher(logger logging.Logger) *MockPublisher {
	return &MockPublisher{
		events: make([]*outbox.OutboxEvent, 0),
		logger: logger,
	}
}

func (mp *MockPublisher) Publish(ctx context.Context, event *outbox.OutboxEvent) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	mp.events = append(mp.events, event)
	mp.logger.Debug(ctx, "Event published to mock",
		"event_id", event.ID().String(),
		"event_type", event.EventType(),
	)
	return nil
}

func (mp *MockPublisher) PublishBatch(ctx context.Context, events []*outbox.OutboxEvent) error {
	for _, event := range events {
		if err := mp.Publish(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (mp *MockPublisher) GetPublishedEvents() []*outbox.OutboxEvent {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	result := make([]*outbox.OutboxEvent, len(mp.events))
	copy(result, mp.events)
	return result
}

func (mp *MockPublisher) ClearPublishedEvents() {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.events = make([]*outbox.OutboxEvent, 0)
}
