package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/settlement-engine/internal/application/services"
)

type CreateAccountRequest struct {
	CustomerID     string `json:"customer_id"`
	InitialBalance int64  `json:"initial_balance"`
}

func (h *Handler) HandleCreateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST allowed", "")
		return
	}

	requestID := r.Context().Value("request_id").(string)
	contextLogger := h.logger.WithRequestID(requestID)

	var req CreateAccountRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		contextLogger.Error(r.Context(), "Failed to decode request", err.Error())
		WriteError(w, http.StatusBadRequest, "invalid_request", "Failed to decode request body", requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	resp, err := h.settlementService.CreateAccount(ctx, services.CreateAccountRequest{
		CustomerID:     req.CustomerID,
		InitialBalance: req.InitialBalance,
	})

	if err != nil {
		contextLogger.Error(r.Context(), "Failed to create account", err.Error())
		status, errorCode := serviceError(err)
		WriteError(w, status, errorCode, err.Error(), requestID)
		return
	}

	contextLogger.Info(r.Context(), "Account created successfully", resp.AccountID.String())
	WriteJSON(w, http.StatusCreated, resp, requestID)
}

func (h *Handler) HandleGetAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET allowed", "")
		return
	}

	requestID := r.Context().Value("request_id").(string)
	contextLogger := h.logger.WithRequestID(requestID)

	accountIDStr := r.URL.Query().Get("id")
	if accountIDStr == "" {
		WriteError(w, http.StatusBadRequest, "missing_parameter", "Account ID is required", requestID)
		return
	}

	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid account ID format", requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	resp, err := h.settlementService.GetAccount(ctx, accountID)
	if err != nil {
		contextLogger.Error(r.Context(), "Failed to get account", err.Error())
		status, errorCode := serviceError(err)
		if status == http.StatusInternalServerError {
			status = http.StatusNotFound
			errorCode = "account_not_found"
		}
		WriteError(w, status, errorCode, err.Error(), requestID)
		return
	}

	WriteJSON(w, http.StatusOK, resp, requestID)
}

type ProcessTransactionRequest struct {
	IdempotencyKey  string `json:"idempotency_key"`
	SourceAccountID string `json:"source_account_id"`
	TargetAccountID string `json:"target_account_id"`
	Amount          int64  `json:"amount"`
	Description     string `json:"description"`
}

func (h *Handler) HandleProcessTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST allowed", "")
		return
	}

	requestID := r.Context().Value("request_id").(string)
	contextLogger := h.logger.WithRequestID(requestID)

	var req ProcessTransactionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		contextLogger.Error(r.Context(), "Failed to decode request", err.Error())
		WriteError(w, http.StatusBadRequest, "invalid_request", "Failed to decode request body", requestID)
		return
	}

	sourceAccountID, err := uuid.Parse(req.SourceAccountID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid source account ID", requestID)
		return
	}

	targetAccountID, err := uuid.Parse(req.TargetAccountID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid target account ID", requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	resp, err := h.settlementService.ProcessTransaction(ctx, services.ProcessTransactionRequest{
		IdempotencyKey:  req.IdempotencyKey,
		SourceAccountID: sourceAccountID,
		TargetAccountID: targetAccountID,
		Amount:          req.Amount,
		Description:     req.Description,
	})

	if err != nil {
		contextLogger.Error(r.Context(), "Failed to process transaction", err.Error())
		status, errorCode := serviceError(err)
		if status == http.StatusInternalServerError {
			status = http.StatusBadRequest
			errorCode = "transaction_failed"
		}
		WriteError(w, status, errorCode, err.Error(), requestID)
		return
	}

	statusCode := http.StatusOK
	if resp.Status == "FAILED" {
		statusCode = http.StatusBadRequest
	}

	contextLogger.Info(r.Context(), "Transaction processed",
		"status", resp.Status, "transaction_id", resp.TransactionID.String())
	WriteJSON(w, statusCode, resp, requestID)
}

func (h *Handler) HandleGetTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET allowed", "")
		return
	}

	requestID := r.Context().Value("request_id").(string)

	transactionIDStr := r.URL.Query().Get("id")
	if transactionIDStr == "" {
		WriteError(w, http.StatusBadRequest, "missing_parameter", "Transaction ID is required", requestID)
		return
	}

	transactionID, err := uuid.Parse(transactionIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_uuid", "Invalid transaction ID format", requestID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	resp, err := h.settlementService.GetTransaction(ctx, transactionID)
	if err != nil {
		status, errorCode := serviceError(err)
		if status == http.StatusInternalServerError {
			status = http.StatusNotFound
			errorCode = "transaction_not_found"
		}
		WriteError(w, status, errorCode, err.Error(), requestID)
		return
	}

	WriteJSON(w, http.StatusOK, resp, requestID)
}

type HealthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Version  string `json:"version"`
}

func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET allowed", "")
		return
	}

	requestID := r.Context().Value("request_id").(string)

	health := HealthResponse{
		Status:   "healthy",
		Database: "connected",
		Version:  "1.0.0",
	}

	WriteJSON(w, http.StatusOK, health, requestID)
}
