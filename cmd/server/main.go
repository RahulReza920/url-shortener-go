// Command server runs the URL shortener HTTP API. See SPEC.md for the
// full design and CLAUDE.md for how to run/test it.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"url-shortener/internal/api"
	"url-shortener/internal/auth"
	"url-shortener/internal/config"
	"url-shortener/internal/safebrowsing"
	"url-shortener/internal/store"
)

func main() {
	cfg := config.Load()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("could not connect to redis at %s: %v", cfg.RedisAddr, err)
	}

	st := store.NewFromClient(rdb)

	tokens, err := auth.NewTokenIssuer(cfg.PASETOSecretKeyHex)
	if err != nil {
		log.Fatalf("could not initialize token issuer: %v", err)
	}
	if cfg.PASETOSecretKeyHex == "" {
		log.Println("WARNING: PASETO_SECRET_KEY not set, generated an ephemeral key — all tokens will be invalidated on restart")
	}

	sb := safebrowsing.New(cfg.SafeBrowsingAPIKey)
	if !sb.Enabled() {
		log.Println("SAFE_BROWSING_API_KEY not set — malicious URL screening is disabled")
	}

	srv := api.NewServer(rdb, st, tokens, sb)
	if err := srv.SeedReservedAliases(); err != nil {
		log.Fatalf("could not seed reserved aliases: %v", err)
	}

	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, srv.Routes()); err != nil {
		log.Fatal(err)
	}
}
