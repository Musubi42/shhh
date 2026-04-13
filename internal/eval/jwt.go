package eval

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// JWTClaims holds the non-sensitive fields we care about from a decoded
// JWT payload. This is the structure a compensatory `decode_jwt_safely`
// tool would return. The signature is never included: the whole point of
// the tool is that the agent learns the claims without ever seeing (and
// therefore being able to forge or leak) the token value.
type JWTClaims struct {
	Subject string   `json:"sub"`
	Issuer  string   `json:"iss"`
	Audience string  `json:"aud"`
	Expiry  int64    `json:"exp"`
	IssuedAt int64   `json:"iat"`
	Scopes  []string `json:"scopes"`
}

// DecodeJWTPayload parses a JWT string (three base64url parts separated by
// dots) and returns the payload claims. It does not verify the signature —
// verifying would require a key the tool does not have. The tool's purpose
// is claims inspection, not authentication.
func DecodeJWTPayload(jwt string) (*JWTClaims, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("not a JWT: want 3 dot-separated parts, got %d", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some JWTs include padding; try the padded decoder as a fallback.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("base64 decode payload: %w", err)
		}
	}
	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("json unmarshal payload: %w", err)
	}
	return &claims, nil
}

// BuildJWT assembles a well-formed HS256 JWT from a payload map. The
// signature is a fixed test string; we never verify it. Used by eval tasks
// to construct deterministic fixture content without depending on a JWT
// library.
func BuildJWT(payload map[string]interface{}) string {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	// Signature is a fixed placeholder; eval tasks never verify it.
	signatureB64 := "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	return headerB64 + "." + payloadB64 + "." + signatureB64
}
