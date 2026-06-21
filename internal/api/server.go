// Package api wires together HTTP handlers, middleware, and the storage/
// auth/safety dependencies for the URL shortener.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"url-shortener/internal/auth"
	"url-shortener/internal/ratelimit"
	"url-shortener/internal/safebrowsing"
	"url-shortener/internal/store"
)

const (
	anonymousCreateLimit     = 10 // creates per rateLimitWindow, anonymous
	authenticatedCreateLimit = 60 // creates per rateLimitWindow, authenticated
	rateLimitWindow          = time.Minute

	aliasMinLen = 3
	aliasMaxLen = 30
)

var reservedAliases = []string{
	"admin", "api", "login", "signup", "www", "root", "static",
	"health", "favicon.ico", "robots.txt", "assets",
}

type Server struct {
	store        *store.Store
	tokens       *auth.TokenIssuer
	limiter      *ratelimit.Limiter
	safeBrowsing *safebrowsing.Client
}

func NewServer(rdb *redis.Client, st *store.Store, tokens *auth.TokenIssuer, sb *safebrowsing.Client) *Server {
	return &Server{
		store:        st,
		tokens:       tokens,
		limiter:      ratelimit.New(rdb, rateLimitWindow),
		safeBrowsing: sb,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/signup", s.handleSignup)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/links", s.optionalAuth(s.rateLimitCreate(s.handleCreateLink)))
	mux.HandleFunc("GET /api/links", s.requireAuth(s.handleListLinks))
	mux.HandleFunc("GET /{code}", s.handleRedirect)
	return mux
}

// SeedReservedAliases must be called once at startup.
func (s *Server) SeedReservedAliases() error {
	return s.store.SeedReservedAliases(bgCtx(), reservedAliases)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
