package cmdhook

import (
	"crypto/sha1"
	"encoding/hex"
)

// hashName is the short stable filename for a redacted copy of a source
// file. SHA-1 is fine here — this is a cache key, not a security primitive.
func hashName(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}
