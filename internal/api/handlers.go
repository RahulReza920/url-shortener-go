package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"url-shortener/internal/auth"
	"url-shortener/internal/codegen"
	"url-shortener/internal/store"
)

func bgCtx() context.Context { return context.Background() }

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// --- signup / login ---

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	id, err := randomID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate user id")
		return
	}

	if err := s.store.CreateUser(r.Context(), id, req.Email, hash); err != nil {
		if errors.Is(err, store.ErrAliasTaken) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"token": s.tokens.Issue(id)})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := s.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	ok, err := auth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"token": s.tokens.Issue(user.ID)})
}

// --- links ---

type createLinkRequest struct {
	URL        string `json:"url"`
	Alias      string `json:"alias,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
}

type createLinkResponse struct {
	Code string `json:"code"`
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	var req createLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	if _, err := url.ParseRequestURI(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, "url is not a valid absolute URL")
		return
	}

	if s.safeBrowsing.Enabled() {
		malicious, err := s.safeBrowsing.IsMalicious(r.Context(), req.URL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not verify url safety")
			return
		}
		if malicious {
			writeError(w, http.StatusBadRequest, "url flagged as malicious")
			return
		}
	}

	ttl := time.Duration(req.TTLSeconds) * time.Second
	ownerID := userIDFromContext(r)

	code := req.Alias
	if code != "" {
		if err := s.validateAlias(r.Context(), code); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		seq, err := s.store.NextSeq(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not generate code")
			return
		}
		code = codegen.Code(seq)
	}

	if err := s.store.CreateLink(r.Context(), code, req.URL, ownerID, ttl); err != nil {
		if errors.Is(err, store.ErrAliasTaken) {
			writeError(w, http.StatusConflict, "alias already taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create link")
		return
	}

	writeJSON(w, http.StatusCreated, createLinkResponse{Code: code})
}

func (s *Server) validateAlias(ctx context.Context, alias string) error {
	if len(alias) < aliasMinLen || len(alias) > aliasMaxLen {
		return errInvalidAlias("alias must be between 3 and 30 characters")
	}
	if !aliasPattern.MatchString(alias) {
		return errInvalidAlias("alias may only contain letters, numbers, and hyphens")
	}
	reserved, err := s.store.IsReservedAlias(ctx, alias)
	if err != nil {
		return err
	}
	if reserved {
		return errInvalidAlias("alias is reserved")
	}
	return nil
}

type errInvalidAlias string

func (e errInvalidAlias) Error() string { return string(e) }

func (s *Server) handleListLinks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r)
	codes, err := s.store.ListUserLinks(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list links")
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"links": codes})
}

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	dest, err := s.store.ResolveAndCount(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	http.Redirect(w, r, dest, http.StatusFound) // 302 — see SPEC.md section 6
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
