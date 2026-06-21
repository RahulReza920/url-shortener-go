package api

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type ctxKey string

const userIDCtxKey ctxKey = "userID"

// optionalAuth attaches the caller's userID to the request context if a
// valid PASETO bearer token is present, but does not reject the request
// otherwise — anonymous creation is allowed (see SPEC.md section 9).
func (s *Server) optionalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token, ok := bearerToken(r); ok {
			if userID, err := s.tokens.Verify(token); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), userIDCtxKey, userID))
			}
		}
		next(w, r)
	}
}

// requireAuth rejects the request with 401 unless a valid token is present.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		userID, err := s.tokens.Verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), userIDCtxKey, userID))
		next(w, r)
	}
}

func userIDFromContext(r *http.Request) string {
	v, _ := r.Context().Value(userIDCtxKey).(string)
	return v
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	return strings.TrimPrefix(h, prefix), true
}

// rateLimitCreate enforces the per-IP limit on link creation only.
func (s *Server) rateLimitCreate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		limit := int64(anonymousCreateLimit)
		if userIDFromContext(r) != "" {
			limit = authenticatedCreateLimit
		}
		ok, err := s.limiter.Allow(r.Context(), ip, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "rate limit check failed")
			return
		}
		if !ok {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next(w, r)
	}
}

func clientIP(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-For"); h != "" {
		return strings.TrimSpace(strings.Split(h, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
