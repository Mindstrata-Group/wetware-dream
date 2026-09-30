// Package httpjson holds the JSON request/response helpers shared by every
// module. It is part of the kernel: no business logic, no module imports.
package httpjson

import (
	"encoding/json"
	"net/http"
)

// Write sends payload as JSON with the given status code.
func Write(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// DecodeStrict reads at most maxBytes of the request body and decodes it into
// v, rejecting unknown fields. An unknown field means the client and the
// server disagree on the contract, and that should fail at the request, not
// somewhere deeper in the code.
func DecodeStrict(w http.ResponseWriter, r *http.Request, maxBytes int64, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
