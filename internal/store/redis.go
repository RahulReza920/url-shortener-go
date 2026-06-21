// Package store wraps Redis access behind the key scheme used by the rest
// of the service. There is deliberately no storage interface/abstraction —
// v1 talks to Redis directly (see SPEC.md, "Out of Scope").
package store

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNotFound = errors.New("not found")
var ErrAliasTaken = errors.New("alias already taken")

type Store struct {
	rdb *redis.Client
}

func New(addr string) *Store {
	return &Store{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

func NewFromClient(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.rdb.Ping(ctx).Err()
}

// NextSeq returns the next value of the global short-code counter.
func (s *Store) NextSeq(ctx context.Context) (uint64, error) {
	n, err := s.rdb.Incr(ctx, "seq:code").Result()
	if err != nil {
		return 0, err
	}
	return uint64(n), nil
}

type LinkMeta struct {
	OwnerID   string
	CreatedAt time.Time
}

// CreateLink atomically claims `code` (fails with ErrAliasTaken if already
// in use — this is also how custom alias collisions are rejected) and
// stores the destination URL plus metadata. ttl of 0 means no expiry.
func (s *Store) CreateLink(ctx context.Context, code, destURL, ownerID string, ttl time.Duration) error {
	ok, err := s.rdb.SetNX(ctx, urlKey(code), destURL, ttl).Result()
	if err != nil {
		return err
	}
	if !ok {
		return ErrAliasTaken
	}

	meta := map[string]any{
		"owner_id":   ownerID,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, metaKey(code), meta)
	if ttl > 0 {
		pipe.Expire(ctx, metaKey(code), ttl)
	}
	if ownerID != "" {
		pipe.SAdd(ctx, userLinksKey(ownerID), code)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// ResolveAndCount looks up the destination for code and increments its
// click counter in the same round trip.
func (s *Store) ResolveAndCount(ctx context.Context, code string) (string, error) {
	url, err := s.rdb.Get(ctx, urlKey(code)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	s.rdb.Incr(ctx, clicksKey(code))
	return url, nil
}

func (s *Store) ClickCount(ctx context.Context, code string) (int64, error) {
	n, err := s.rdb.Get(ctx, clicksKey(code)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return n, err
}

// ListUserLinks returns the codes owned by userID.
func (s *Store) ListUserLinks(ctx context.Context, userID string) ([]string, error) {
	return s.rdb.SMembers(ctx, userLinksKey(userID)).Result()
}

// IsReservedAlias reports whether word is in the reserved-alias blocklist.
func (s *Store) IsReservedAlias(ctx context.Context, word string) (bool, error) {
	return s.rdb.SIsMember(ctx, "alias:reserved", word).Result()
}

// SeedReservedAliases adds the given words to the reserved-alias blocklist.
// Idempotent — safe to call on every startup.
func (s *Store) SeedReservedAliases(ctx context.Context, words []string) error {
	if len(words) == 0 {
		return nil
	}
	members := make([]any, len(words))
	for i, w := range words {
		members[i] = w
	}
	return s.rdb.SAdd(ctx, "alias:reserved", members...).Err()
}

type User struct {
	ID           string
	Email        string
	PasswordHash string
}

// CreateUser atomically claims the email and stores the account. Returns
// ErrAliasTaken if the email is already registered.
func (s *Store) CreateUser(ctx context.Context, id, email, passwordHash string) error {
	ok, err := s.rdb.SetNX(ctx, userEmailKey(email), id, 0).Result()
	if err != nil {
		return err
	}
	if !ok {
		return ErrAliasTaken
	}
	return s.rdb.HSet(ctx, userKey(id), map[string]any{
		"email":         email,
		"password_hash": passwordHash,
	}).Err()
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	id, err := s.rdb.Get(ctx, userEmailKey(email)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	h, err := s.rdb.HGetAll(ctx, userKey(id)).Result()
	if err != nil {
		return nil, err
	}
	if len(h) == 0 {
		return nil, ErrNotFound
	}
	return &User{ID: id, Email: h["email"], PasswordHash: h["password_hash"]}, nil
}

func urlKey(code string) string        { return "url:" + code }
func metaKey(code string) string       { return "url:" + code + ":meta" }
func clicksKey(code string) string     { return "url:" + code + ":clicks" }
func userKey(id string) string         { return "user:" + id }
func userEmailKey(email string) string { return "user:email:" + email }
func userLinksKey(id string) string    { return "user:" + id + ":links" }
