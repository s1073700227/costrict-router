package config

import (
	"strings"
	"testing"
)

func TestLocalAPIKeyHashAndVerify(t *testing.T) {
	apiKey, err := GenerateLocalAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(apiKey, "sk-costrict-") {
		t.Fatalf("api key prefix = %q", apiKey)
	}
	hash, err := HashLocalAPIKey(apiKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(hash, apiKey) {
		t.Fatalf("hash leaked api key: %s", hash)
	}
	if !VerifyLocalAPIKey(apiKey, hash) {
		t.Fatal("expected api key to verify")
	}
	if VerifyLocalAPIKey(apiKey+"x", hash) {
		t.Fatal("expected wrong api key to fail")
	}
	if VerifyLocalAPIKey("", hash) {
		t.Fatal("expected empty api key to fail")
	}
}

func TestSetLocalAPIKey(t *testing.T) {
	apiKey, err := GenerateLocalAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	if err := cfg.SetLocalAPIKey(apiKey); err != nil {
		t.Fatal(err)
	}
	if cfg.LocalAPIKeyHash == "" || cfg.LocalAPIKeyCreatedAt.IsZero() {
		t.Fatalf("local api key fields were not set: %+v", cfg)
	}
	if !cfg.VerifyLocalAPIKey(apiKey) {
		t.Fatal("expected config to verify generated key")
	}
}
