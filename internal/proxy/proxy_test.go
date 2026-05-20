package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"costrict-router/internal/config"
	"costrict-router/internal/logx"
)

type fakeTokens struct {
	cfg config.Config
}

func (f *fakeTokens) Config() config.Config {
	return f.cfg
}

func (f *fakeTokens) EnsureFreshToken(context.Context) error {
	return nil
}

func TestForwardChatAddsCostrictHeaders(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/chat-rag/api/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("zgsm-client-id") != "machine" || r.Header.Get("x-user-id") != "user" {
			t.Fatalf("headers = %+v", r.Header)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
		}, nil
	})}

	handler := &Handler{
		Tokens: &fakeTokens{cfg: config.Config{
			BaseURL:      "https://example.com",
			AccessToken:  "access",
			RefreshToken: "refresh",
			MachineCode:  "machine",
			UserID:       "user",
		}},
		Client: client,
		Logger: logx.New(&strings.Builder{}, false),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"m"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestHealthzRedactsTokens(t *testing.T) {
	handler := &Handler{
		Tokens: &fakeTokens{cfg: config.Config{
			BaseURL:               "https://example.com",
			ListenAddr:            "127.0.0.1:14567",
			AccessToken:           "abcdefghijklmnopqrstuvwxyz",
			RefreshToken:          "refreshabcdefghijklmnopqrstuvwxyz",
			MachineCode:           "machineabcdefghijklmnopqrstuvwxyz",
			AccessTokenExpiresAt:  time.Unix(1893456000, 0),
			RefreshTokenExpiresAt: time.Unix(1893456000, 0),
		}},
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("healthz leaked token-like value: %s", rec.Body.String())
	}
}
