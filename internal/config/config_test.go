package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTokenFileEnvelope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(path, []byte(`{"success":true,"data":{"access_token":"a","refresh_token":"r","state":"s"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tf, err := ReadTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if tf.AccessToken != "a" || tf.RefreshToken != "r" || tf.State != "s" {
		t.Fatalf("token file = %+v", tf)
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := Default()
	cfg.BaseURL = "https://example.com"
	cfg.AccessToken = "access"
	cfg.RefreshToken = "refresh"
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.BaseURL != cfg.BaseURL || loaded.MachineCode == "" {
		t.Fatalf("loaded = %+v", loaded)
	}
}
