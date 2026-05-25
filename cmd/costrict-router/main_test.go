package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"costrict-router/internal/config"
)

func TestEnsureLocalAPIKeyGeneratesAndDoesNotRepeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()

	var out bytes.Buffer
	apiKey, err := ensureLocalAPIKey(path, &cfg, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(apiKey, "sk-costrict-") {
		t.Fatalf("api key = %q", apiKey)
	}
	if !strings.Contains(out.String(), apiKey) {
		t.Fatalf("api key was not printed: %q", out.String())
	}
	if cfg.LocalAPIKeyHash == "" || strings.Contains(cfg.LocalAPIKeyHash, apiKey) {
		t.Fatalf("hash was not stored safely: %q", cfg.LocalAPIKeyHash)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.VerifyLocalAPIKey(apiKey) {
		t.Fatal("saved config does not verify generated api key")
	}

	out.Reset()
	second, err := ensureLocalAPIKey(path, loaded, &out)
	if err != nil {
		t.Fatal(err)
	}
	if second != "" || out.Len() != 0 {
		t.Fatalf("expected existing key to be left alone, key=%q out=%q", second, out.String())
	}
}

func TestResetLocalAPIKeyReplacesOldKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()

	oldKey, err := resetLocalAPIKey(path, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	newKey, err := resetLocalAPIKey(path, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if oldKey == newKey {
		t.Fatal("reset returned the same api key")
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VerifyLocalAPIKey(oldKey) {
		t.Fatal("old api key still verifies after reset")
	}
	if !loaded.VerifyLocalAPIKey(newKey) {
		t.Fatal("new api key does not verify after reset")
	}
}
