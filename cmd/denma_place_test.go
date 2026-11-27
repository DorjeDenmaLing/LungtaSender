package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
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

// TestDenmaOldCenterPage: a page load of center content at the hub's address goes to sign-in with
// the path as next; the hub's own pages, the Media picker and non-page loads are left to the hub.
func TestDenmaOldCenterPage(t *testing.T) {
	for _, tc := range []struct {
		method, path, mode, cookie, want string
	}{
		{"GET", "/admin/campaigns/5", "navigate", "", "/login?next=%2Fadmin%2Fcampaigns%2F5"},
		{"GET", "/admin/lists", "navigate", "", "/login?next=%2Fadmin%2Flists"},
		{"GET", "/admin/campaigns/5", "navigate", "session", "/admin/centers"},
		{"GET", "/admin/campaigns/5", "", "", ""},
		{"POST", "/admin/campaigns/5", "navigate", "", ""},
		{"GET", "/api/lists", "navigate", "", ""},
		{"GET", "/admin/campaigns/media", "navigate", "", ""},
		{"GET", "/admin/centers", "navigate", "", ""},
		{"GET", "/admin", "navigate", "", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.mode != "" {
			req.Header.Set("Sec-Fetch-Mode", tc.mode)
		}
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: tc.cookie, Value: "x"})
		}
		c := echo.New().NewContext(req, httptest.NewRecorder())
		if got := denmaOldCenterPage(c, tc.path); got != tc.want {
			t.Errorf("%s %s (%s): got %q, want %q", tc.method, tc.path, tc.cookie, got, tc.want)
		}
	}
}

// TestDenmaLanding: signing in to a center lands on the admin page wanted, under the center's address, and never elsewhere.
func TestDenmaLanding(t *testing.T) {
	ctr := &denmaCenter{Slug: "ddl", app: &App{urlCfg: &UrlConfig{RootPath: "/c/ddl"}}}
	for next, want := range map[string]string{
		"/admin/campaigns/5":     "/c/ddl/admin/campaigns/5",
		"/c/ddl/admin/lists":     "/c/ddl/admin/lists",
		"/c/halifax/admin/lists": "/c/ddl/admin",
		"/admin":                 "/c/ddl/admin",
		"/":                      "/c/ddl/admin",
		"/api/lists":             "/c/ddl/admin",
		"/login/centers":         "/c/ddl/admin",
	} {
		if got := denmaLanding(ctr, next); got != want {
			t.Errorf("%s: got %s, want %s", next, got, want)
		}
	}
}
