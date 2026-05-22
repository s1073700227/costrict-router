package config

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	localAPIKeyPrefix = "sk-costrict-"
	localAPIKeyFormat = "v1:sha256"
)

func GenerateLocalAPIKey() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("生成本地 API Key 失败: %w", err)
	}
	return localAPIKeyPrefix + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func HashLocalAPIKey(apiKey string) (string, error) {
	if !strings.HasPrefix(apiKey, localAPIKeyPrefix) {
		return "", errors.New("本地 API Key 格式无效")
	}
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", fmt.Errorf("生成本地 API Key salt 失败: %w", err)
	}
	digest := digestLocalAPIKey(salt[:], apiKey)
	return strings.Join([]string{
		localAPIKeyFormat,
		base64.RawURLEncoding.EncodeToString(salt[:]),
		base64.RawURLEncoding.EncodeToString(digest),
	}, ":"), nil
}

func VerifyLocalAPIKey(apiKey, hash string) bool {
	if apiKey == "" || hash == "" {
		return false
	}
	parts := strings.Split(hash, ":")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "sha256" {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(want) != sha256.Size {
		return false
	}
	got := digestLocalAPIKey(salt, apiKey)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (c *Config) SetLocalAPIKey(apiKey string) error {
	hash, err := HashLocalAPIKey(apiKey)
	if err != nil {
		return err
	}
	c.LocalAPIKeyHash = hash
	c.LocalAPIKeyCreatedAt = time.Now()
	return nil
}

func (c Config) VerifyLocalAPIKey(apiKey string) bool {
	return VerifyLocalAPIKey(apiKey, c.LocalAPIKeyHash)
}

func digestLocalAPIKey(salt []byte, apiKey string) []byte {
	h := sha256.New()
	_, _ = h.Write(salt)
	_, _ = h.Write([]byte(apiKey))
	return h.Sum(nil)
}
