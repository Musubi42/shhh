package audit

import "encoding/json"

// jsonUnmarshal is a tiny indirection around json.Unmarshal so
// run.go can call it without a direct encoding/json import (keeping
// the run.go dependency surface visible at the imports list). This
// is cosmetic — inline if the indirection grows distracting.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
