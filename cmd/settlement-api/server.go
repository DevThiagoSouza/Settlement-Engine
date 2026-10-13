package main

import (
	"context"
	"database/sql"
	"fmt"
	stdhttp "net/http"
	"time"

	_ "github.com/lib/pq"
	"github.com/settlement-engine/internal/application/services"
	"github.com/settlement-engine/internal/config"
	databaseinfra "github.com/settlement-engine/internal/infrastructure/database"
	httpinfra "github.com/settlement-engine/internal/infrastructure/http"
	"github.com/settlement-engine/internal/infrastructure/logging"
	"github.com/settlement-engine/internal/infrastructure/messaging/kafka"
	"github.com/settlement-engine/internal/infrastructure/outbox"
	"github.com/settlement-engine/internal/infrastructure/persistence"
	"github.com/settlement-engine/internal/infrastructure/reconciliation"
)

func main() {
	cfg := config.Load()

	logger := logging.NewLogger(logging.INFO)

	logger.Info(context.Background(), "Starting Settlement Engine API",
		"environment", cfg.Environment,
		"port", cfg.HTTPPort,
	)

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		logger.Fatal(context.Background(), "Failed to open database connection", err.Error())
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		logger.Fatal(context.Background(), "Failed to ping database", err.Error())
	}

	logger.Info(context.Background(), "Database connected successfully")

	accountRepo := persistence.NewPostgresAccountRepository(db)
	transactionRepo := persistence.NewPostgresTransactionRepository(db)
	outboxRepo := persistence.NewPostgresOutboxRepository(db)

	kafkaPublisher := kafka.NewKafkaPublisherWithConnection(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaConnectionString, logger)

	unitOfWork := databaseinfra.NewPostgresUnitOfWork(db)
	settlementService := services.NewSettlementService(accountRepo, transactionRepo, outboxRepo, unitOfWork)
	reconciliationService := reconciliation.NewReconciliationService(accountRepo, transactionRepo)

	outboxProcessor := outbox.NewOutboxProcessor(outboxRepo, kafkaPublisher)

	handler := httpinfra.NewHandler(settlementService, reconciliationService, logger)

	router := httpinfra.NewRouter(handler, logger, cfg.JWTSecret)
	apiHandler := router.Setup()

	server := &stdhttp.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.HTTPHost, cfg.HTTPPort),
		Handler:      apiHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			processedCount, err := outboxProcessor.ProcessPendingEvents(context.Background(), 100)
			if err != nil {
				logger.Error(context.Background(), "Error processing outbox events", err.Error())
			} else if processedCount > 0 {
				logger.Debug(context.Background(), "Processed outbox events", processedCount)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			olderThan := time.Now().Add(-24 * time.Hour)
			if err := outboxProcessor.CleanupOldEvents(context.Background(), olderThan); err != nil {
				logger.Error(context.Background(), "Error cleaning up old events", err.Error())
			}
		}
	}()

	logger.Info(context.Background(), "HTTP server starting",
		"address", server.Addr,
		"kafka_brokers", cfg.KafkaBrokers,
		"kafka_topic", cfg.KafkaTopic,
	)

	if err := server.ListenAndServe(); err != nil && err != stdhttp.ErrServerClosed {
		logger.Fatal(context.Background(), "Server error", err.Error())
	}
}
