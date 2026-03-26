package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.AuthToken == "" {
			next(w, r)
			return
		}

		header := strings.TrimSpace(r.Header.Get("Authorization"))
		if header == "" {
			writeError(w, http.StatusUnauthorized, "missing_token", "missing Authorization bearer token")
			return
		}

		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			writeError(w, http.StatusUnauthorized, "invalid_auth_scheme", "Authorization must use Bearer scheme")
			return
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(token)), []byte(a.cfg.AuthToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid_token", "bearer token is invalid")
			return
		}

		next(w, r)
	}
}
