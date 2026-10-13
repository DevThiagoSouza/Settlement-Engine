package http

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/settlement-engine/internal/application/ports"
)

func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				WriteError(w, http.StatusInternalServerError, "authentication_not_configured", "Authentication is not configured", requestIDFromContext(r))
				return
			}

			const prefix = "Bearer "
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, prefix) {
				WriteError(w, http.StatusUnauthorized, "missing_token", "Bearer token is required", requestIDFromContext(r))
				return
			}

			token, err := jwt.Parse(strings.TrimSpace(strings.TrimPrefix(header, prefix)), func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				WriteError(w, http.StatusUnauthorized, "invalid_token", "Bearer token is invalid", requestIDFromContext(r))
				return
			}

			subject, err := token.Claims.GetSubject()
			if err != nil || subject == "" {
				WriteError(w, http.StatusUnauthorized, "missing_subject", "Token subject is required", requestIDFromContext(r))
				return
			}

			next.ServeHTTP(w, r.WithContext(ports.WithPrincipal(r.Context(), subject)))
		})
	}
}

func requestIDFromContext(r *http.Request) string {
	requestID, _ := r.Context().Value("request_id").(string)
	return requestID
}
