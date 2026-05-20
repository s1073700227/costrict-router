package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestDecodeClaims(t *testing.T) {
	token := makeUnsignedJWT(map[string]any{
		"exp":          int64(1893456000),
		"iat":          int64(1893455900),
		"universal_id": "u1",
		"sub":          "s1",
	})
	claims, err := DecodeClaims(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UniversalID != "u1" || claims.Subject != "s1" {
		t.Fatalf("claims = %+v", claims)
	}
	if !claims.ExpiresAt.Equal(time.Unix(1893456000, 0)) {
		t.Fatalf("expires = %s", claims.ExpiresAt)
	}
}

func makeUnsignedJWT(payload map[string]any) string {
	header, _ := json.Marshal(map[string]any{"alg": "none"})
	body, _ := json.Marshal(payload)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body) + "."
}
