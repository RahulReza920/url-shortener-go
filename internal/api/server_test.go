package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"url-shortener/internal/auth"
	"url-shortener/internal/safebrowsing"
	"url-shortener/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	st := store.NewFromClient(rdb)
	tokens, err := auth.NewTokenIssuer("")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	srv := NewServer(rdb, st, tokens, safebrowsing.New(""))
	if err := srv.SeedReservedAliases(); err != nil {
		t.Fatalf("SeedReservedAliases: %v", err)
	}
	return srv
}

func TestCreateAndRedirect_EndToEnd(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	createBody, _ := json.Marshal(createLinkRequest{URL: "https://example.com/some/long/path"})
	resp, err := http.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created createLinkResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	resp.Body.Close()
	if created.Code == "" {
		t.Fatal("expected non-empty code")
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	redirectResp, err := client.Get(ts.URL + "/" + created.Code)
	if err != nil {
		t.Fatalf("redirect request failed: %v", err)
	}
	defer redirectResp.Body.Close()

	if redirectResp.StatusCode != http.StatusFound {
		t.Fatalf("expected 302, got %d", redirectResp.StatusCode)
	}
	if loc := redirectResp.Header.Get("Location"); loc != "https://example.com/some/long/path" {
		t.Fatalf("expected redirect to original URL, got %q", loc)
	}

	count, err := srv.store.ClickCount(t.Context(), created.Code)
	if err != nil {
		t.Fatalf("ClickCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected click count 1, got %d", count)
	}
}

func TestRedirect_UnknownCode404(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/doesnotexist")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestCustomAlias_ReservedWordRejected(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(createLinkRequest{URL: "https://example.com", Alias: "admin"})
	resp, err := http.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for reserved alias, got %d", resp.StatusCode)
	}
}

func TestCustomAlias_DuplicateRejected(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	body, _ := json.Marshal(createLinkRequest{URL: "https://example.com", Alias: "my-brand"})
	resp1, err := http.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("first create request failed: %v", err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first create expected 201, got %d", resp1.StatusCode)
	}

	resp2, err := http.Post(ts.URL+"/api/links", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("duplicate create request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate alias expected 409, got %d", resp2.StatusCode)
	}
}

func TestSignupLoginAndListLinks(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	signupBody, _ := json.Marshal(signupRequest{Email: "a@example.com", Password: "hunter2hunter2"})
	resp, err := http.Post(ts.URL+"/api/signup", "application/json", bytes.NewReader(signupBody))
	if err != nil {
		t.Fatalf("signup failed: %v", err)
	}
	var signupResp map[string]string
	json.NewDecoder(resp.Body).Decode(&signupResp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	token := signupResp["token"]
	if token == "" {
		t.Fatal("expected token in signup response")
	}

	createBody, _ := json.Marshal(createLinkRequest{URL: "https://example.com/owned"})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/links", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create with auth failed: %v", err)
	}
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/links", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list links failed: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listed map[string][]string
	json.NewDecoder(listResp.Body).Decode(&listed)
	if len(listed["links"]) != 1 {
		t.Fatalf("expected 1 owned link, got %d", len(listed["links"]))
	}
}

func TestListLinks_RequiresAuth(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/links")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
