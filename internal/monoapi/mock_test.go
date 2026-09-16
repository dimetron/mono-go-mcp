package monoapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newServer starts an httptest server running handler and stops it when
// the test ends.
func newServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// writeJSON writes v as an application/json response body.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
