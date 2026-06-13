package analyzer

import "encoding/json"

// jsonMarshal is a small wrapper around encoding/json that
// panics on error. Used by tests where a Marshal failure is a
// programming error (the type is always under our control).
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
