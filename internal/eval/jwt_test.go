package eval

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestBuildAndDecodeJWT(t *testing.T) {
	original := map[string]interface{}{
		"sub":    "alice@example.com",
		"exp":    int64(1999999999),
		"iat":    int64(1516239022),
		"scopes": []string{"read", "write"},
	}
	jwt := BuildJWT(original)

	claims, err := DecodeJWTPayload(jwt)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if claims.Subject != "alice@example.com" {
		t.Errorf("sub = %q, want alice@example.com", claims.Subject)
	}
	if claims.Expiry != 1999999999 {
		t.Errorf("exp = %d, want 1999999999", claims.Expiry)
	}
	if len(claims.Scopes) != 2 || claims.Scopes[0] != "read" || claims.Scopes[1] != "write" {
		t.Errorf("scopes = %v, want [read write]", claims.Scopes)
	}
}

func TestDecodeJWTPayload_AudienceShapes(t *testing.T) {
	// RFC 7519 §4.1.3 allows `aud` to be either a string or an array of
	// strings. Both must decode into JWTClaims.Audience as a slice.
	mk := func(payload map[string]interface{}) string {
		body, _ := json.Marshal(payload)
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
		return header + "." + base64.RawURLEncoding.EncodeToString(body) + ".sig"
	}

	cases := []struct {
		name    string
		payload map[string]interface{}
		want    []string
	}{
		{
			name:    "single string audience",
			payload: map[string]interface{}{"sub": "a", "aud": "api.example.com"},
			want:    []string{"api.example.com"},
		},
		{
			name:    "array audience",
			payload: map[string]interface{}{"sub": "a", "aud": []string{"api.example.com", "billing.example.com"}},
			want:    []string{"api.example.com", "billing.example.com"},
		},
		{
			name:    "absent audience",
			payload: map[string]interface{}{"sub": "a"},
			want:    nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := DecodeJWTPayload(mk(tc.payload))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(claims.Audience) != len(tc.want) {
				t.Fatalf("aud = %v, want %v", claims.Audience, tc.want)
			}
			for i := range tc.want {
				if claims.Audience[i] != tc.want[i] {
					t.Errorf("aud[%d] = %q, want %q", i, claims.Audience[i], tc.want[i])
				}
			}
		})
	}
}

func TestDecodeJWTPayload_Malformed(t *testing.T) {
	cases := []string{
		"not a jwt",
		"only.two",
		"three.parts.but^^^invalid_base64",
		"eyJhbGciOiJIUzI1NiJ9.bm90LWpzb24.sig", // payload base64-decodes to "not-json"
	}
	for _, c := range cases {
		if claims, err := DecodeJWTPayload(c); err == nil {
			t.Errorf("expected error on %q, got claims=%+v", c, claims)
		}
	}
}
