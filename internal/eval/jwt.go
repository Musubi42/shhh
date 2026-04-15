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
//
// Audience is always decoded as a slice because RFC 7519 §4.1.3 allows
// `aud` to be either a single string or an array of strings, and real
// IdPs (Google, Auth0, Okta) ship array audiences for multi-resource
// tokens. UnmarshalJSON normalizes both shapes into []string.
type JWTClaims struct {
	Subject  string   `json:"sub"`
	Issuer   string   `json:"iss"`
	Audience []string `json:"aud"`
	Expiry   int64    `json:"exp"`
	IssuedAt int64    `json:"iat"`
	Scopes   []string `json:"scopes"`
}

// UnmarshalJSON decodes a JWT payload and normalizes `aud` to []string
// regardless of whether the payload encodes it as a string or an array.
func (c *JWTClaims) UnmarshalJSON(data []byte) error {
	type rawClaims struct {
		Subject  string          `json:"sub"`
		Issuer   string          `json:"iss"`
		Audience json.RawMessage `json:"aud"`
		Expiry   int64           `json:"exp"`
		IssuedAt int64           `json:"iat"`
		Scopes   []string        `json:"scopes"`
	}
	var raw rawClaims
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Subject = raw.Subject
	c.Issuer = raw.Issuer
	c.Expiry = raw.Expiry
	c.IssuedAt = raw.IssuedAt
	c.Scopes = raw.Scopes
	if len(raw.Audience) > 0 {
		var arr []string
		if err := json.Unmarshal(raw.Audience, &arr); err == nil {
			c.Audience = arr
		} else {
			var single string
			if err := json.Unmarshal(raw.Audience, &single); err != nil {
				return fmt.Errorf("aud must be string or []string: %w", err)
			}
			c.Audience = []string{single}
		}
	}
	return nil
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
