package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/application/services"
	"github.com/settlement-engine/internal/infrastructure/logging"
)

type ErrorResponse struct {
	Error      string    `json:"error"`
	Message    string    `json:"message"`
	StatusCode int       `json:"status_code"`
	Timestamp  time.Time `json:"timestamp"`
	RequestID  string    `json:"request_id,omitempty"`
}

type SuccessResponse struct {
	Data      interface{} `json:"data"`
	Message   string      `json:"message,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

type Handler struct {
	settlementService     *services.SettlementService
	reconciliationService interface{}
	logger                logging.Logger
}

func NewHandler(
	settlementService *services.SettlementService,
	reconciliationService interface{},
	logger logging.Logger,
) *Handler {
	return &Handler{
		settlementService:     settlementService,
		reconciliationService: reconciliationService,
		logger:                logger,
	}
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}, requestID string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := SuccessResponse{
		Data:      data,
		Timestamp: time.Now().UTC(),
		RequestID: requestID,
	}

	return json.NewEncoder(w).Encode(response)
}

func WriteError(w http.ResponseWriter, status int, errorMsg, message, requestID string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := ErrorResponse{
		Error:      errorMsg,
		Message:    message,
		StatusCode: status,
		Timestamp:  time.Now().UTC(),
		RequestID:  requestID,
	}

	return json.NewEncoder(w).Encode(response)
}

func serviceError(err error) (int, string) {
	if errors.Is(err, services.ErrForbidden) {
		return http.StatusForbidden, "forbidden"
	}
	return http.StatusInternalServerError, "internal_error"
}

func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoggingMiddleware(logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Context().Value("request_id").(string)
			contextLogger := logger.WithRequestID(requestID)

			contextLogger.Info(r.Context(), "Request received",
				r.Method, r.URL.Path, r.RemoteAddr)

			start := time.Now()
			next.ServeHTTP(w, r)
			duration := time.Since(start)

			contextLogger.Info(r.Context(), "Request completed",
				duration.String())
		})
	}
}

func RecoveryMiddleware(logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := r.Context().Value("request_id").(string)
					contextLogger := logger.WithRequestID(requestID)
					contextLogger.Error(r.Context(), "Panic recovered", err)

					WriteError(w, http.StatusInternalServerError,
						"internal_server_error",
						"An unexpected error occurred",
						requestID)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
