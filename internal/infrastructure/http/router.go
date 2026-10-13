package http

import (
	"net/http"

	"github.com/settlement-engine/internal/infrastructure/logging"
)

type Router struct {
	handler   *Handler
	logger    logging.Logger
	mux       *http.ServeMux
	jwtSecret string
}

func NewRouter(handler *Handler, logger logging.Logger, jwtSecret ...string) *Router {
	secret := ""
	if len(jwtSecret) > 0 {
		secret = jwtSecret[0]
	}
	return &Router{
		handler:   handler,
		logger:    logger,
		mux:       http.NewServeMux(),
		jwtSecret: secret,
	}
}

func (r *Router) Setup() http.Handler {
	globalMiddleware := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mux.ServeHTTP(w, req)
	})

	handler := RecoveryMiddleware(r.logger)(globalMiddleware)
	handler = LoggingMiddleware(r.logger)(handler)
	handler = RequestIDMiddleware(handler)

	r.mux.HandleFunc("/health", r.handler.HandleHealth)
	r.mux.HandleFunc("/readiness", r.handler.HandleHealth)
	r.mux.HandleFunc("/liveness", r.handler.HandleHealth)

	protected := JWTMiddleware(r.jwtSecret)
	r.mux.Handle("/api/v1/accounts", protected(http.HandlerFunc(r.handler.HandleCreateAccount)))
	r.mux.Handle("/api/v1/accounts/get", protected(http.HandlerFunc(r.handler.HandleGetAccount)))

	r.mux.Handle("/api/v1/transactions", protected(http.HandlerFunc(r.handler.HandleProcessTransaction)))
	r.mux.Handle("/api/v1/transactions/get", protected(http.HandlerFunc(r.handler.HandleGetTransaction)))

	r.mux.HandleFunc("/api/v1/info", func(w http.ResponseWriter, req *http.Request) {
		info := map[string]interface{}{
			"name":    "Settlement Engine",
			"version": "1.0.0",
			"api":     "v1",
		}
		WriteJSON(w, http.StatusOK, info, "")
	})

	return handler
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}
