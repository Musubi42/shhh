package eval

import "testing"

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
