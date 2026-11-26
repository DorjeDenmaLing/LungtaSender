package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDenmaPrefixWriter: a center's redirects to its own paths get its prefix;
// those to another center's address or to sign-in are kept.
func TestDenmaPrefixWriter(t *testing.T) {
	for loc, want := range map[string]string{
		"/admin/campaigns/5":    "/c/ddl/admin/campaigns/5",
		"/c/ddl/admin/login":    "/c/ddl/admin/login",
		"/c/halifax/admin":      "/c/halifax/admin",
		"/login":                "/login",
		"/login?next=%2Fadmin":  "/login?next=%2Fadmin",
		"/login/centers":        "/login/centers",
		"/loginx":               "/c/ddl/loginx",
		"//example.org/x":       "//example.org/x",
		"https://example.org/x": "https://example.org/x",
	} {
		rec := httptest.NewRecorder()
		w := &denmaPrefixWriter{ResponseWriter: rec, prefix: "/c/ddl"}
		w.Header().Set("Location", loc)
		w.WriteHeader(http.StatusFound)
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("%s: got %s, want %s", loc, got, want)
		}
	}
}
