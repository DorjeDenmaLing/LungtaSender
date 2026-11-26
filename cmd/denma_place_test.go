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

// TestDenmaNoStorePath: the admin, its API and sign-in are kept by no cache;
// their static files and the public pages may be.
func TestDenmaNoStorePath(t *testing.T) {
	for p, want := range map[string]bool{
		"/admin": true, "/admin/lists": true, "/api/lists": true, "/api/subscribers/export": true,
		"/login": true, "/login/centers": true,
		"/admin/static/style.css": false, "/admin/custom.css": false, "/admin/custom.js": false,
		"/subscription/form": false, "/public/static/style.css": false, "/loginx": false, "/apix": false,
	} {
		if got := denmaNoStorePath(p); got != want {
			t.Errorf("%s: got %v, want %v", p, got, want)
		}
	}
}
