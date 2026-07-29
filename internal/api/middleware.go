package api

import (
	"net/http"
	"strings"
)

// authMiddleware extracts and validates the bearer token.
func (h *Handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			writeError(w, r, http.StatusUnauthorized,
				"missing auth token",
				"call POST /auth/request with email to get an OTP, then POST /auth/verify to get a bearer token")
			return
		}

		wsHandle, err := h.auth.ValidateToken(token)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized,
				"invalid or expired token",
				"call POST /auth/request with email to get a new OTP, then POST /auth/verify to get a bearer token")
			return
		}

		// Store workspace handle in context
		ctx := contextWithWorkspace(r.Context(), wsHandle)
		next(w, r.WithContext(ctx))
	}
}

// extractToken gets the bearer token from the Authorization header.
func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
