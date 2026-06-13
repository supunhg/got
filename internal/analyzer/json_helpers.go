package analyzer

import "encoding/json"

// jsonUnmarshal is a thin wrapper around encoding/json's Unmarshal
// that exists so other files in the package can stay small and
// uniform (the same pattern as `readFile` in walk.go).
//
// We keep this in a dedicated file so it can be swapped for a
// strict or streaming decoder in a future version without
// touching the call sites.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
