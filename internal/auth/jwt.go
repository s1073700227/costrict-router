package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Claims struct {
	ExpiresAt   time.Time
	IssuedAt    time.Time
	UniversalID string
	Subject     string
	UserID      string
}

func DecodeClaims(token string) (Claims, error) {
	// 只解析 JWT payload，不校验签名；这里用于读取过期时间和用户标识。
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return Claims{}, errors.New("JWT 格式无效")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, err
	}

	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return Claims{}, err
	}

	claims := Claims{
		UniversalID: stringValue(raw["universal_id"]),
		Subject:     stringValue(raw["sub"]),
		UserID:      stringValue(raw["id"]),
	}
	if exp, ok := numberValue(raw["exp"]); ok {
		claims.ExpiresAt = time.Unix(exp, 0)
	}
	if iat, ok := numberValue(raw["iat"]); ok {
		claims.IssuedAt = time.Unix(iat, 0)
	}
	return claims, nil
}

func ExpiresWithin(token string, d time.Duration) bool {
	claims, err := DecodeClaims(token)
	if err != nil || claims.ExpiresAt.IsZero() {
		return true
	}
	return time.Until(claims.ExpiresAt) <= d
}

func IsExpired(token string) bool {
	claims, err := DecodeClaims(token)
	if err != nil || claims.ExpiresAt.IsZero() {
		return true
	}
	return time.Now().After(claims.ExpiresAt)
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func numberValue(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}
