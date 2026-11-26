package main

// denma: each center's admin at /c/<slug>/admin; /admin and /api are the hub's, or the place cookie's center.

import (
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/knadh/koanf/v2"
	"github.com/labstack/echo/v4"
)

// denmaPlaceCookie holds the center last opened ("" or absent: the hub).
const denmaPlaceCookie = "denma_center"

// denmaHubSessionCookie is the hub's session cookie.
const denmaHubSessionCookie = "session"

// denmaCookiePath is the session cookies' path: "/" in multi-center mode, else the root URL's path.
func denmaCookiePath(ko *koanf.Koanf, rootPath string) string {
	if ko.Bool("denma.multi_center") || ko.String("denma.center") != "" {
		return "/"
	}
	return rootPath
}

// denmaCenterAdmin is a center's admin address, /c/<slug>/admin.
func denmaCenterAdmin(ctr *denmaCenter) string {
	return path.Join(ctr.app.urlCfg.RootPath, uriAdmin)
}

// denmaCookieName is a session cookie's name in an App: a center's has its slug.
func denmaCookieName(ko *koanf.Koanf, name string) string {
	if slug := ko.String("denma.center"); slug != "" {
		return name + "_" + slug
	}
	return name
}

// denmaSetPlace makes the admin show a center (slug), or the hub ("").
func denmaSetPlace(c echo.Context, slug string) {
	ck := &http.Cookie{
		Name:     denmaPlaceCookie,
		Value:    slug,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Scheme() == "https" || c.Request().Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   int((7 * 24 * time.Hour).Seconds()),
	}
	if slug == "" {
		ck.MaxAge = -1
	}
	c.SetCookie(ck)
}

// denmaPlace is the center the admin shows, or "" for the hub.
func denmaPlace(c echo.Context) string {
	ck, err := c.Cookie(denmaPlaceCookie)
	if err != nil {
		return ""
	}
	return ck.Value
}

// denmaAdminRequest reports whether a path is the admin's or its API's.
func denmaAdminRequest(p string) bool {
	return p == uriAdmin || strings.HasPrefix(p, uriAdmin+"/") || p == "/api" || strings.HasPrefix(p, "/api/")
}

// denmaHubOnlyPath reports whether an admin path is the hub's alone.
func denmaHubOnlyPath(p string) bool {
	// The hub's Analytics; a center's pages are under it (cmd/denma_analytics.go).
	if denmaCenterSettingsPath(p) || p == "/admin/analytics" {
		return true
	}
	for _, pre := range []string{
		"/admin/hub", "/admin/centers", "/admin/people", "/admin/domains", "/admin/system", "/admin/mcp", "/admin/webhooks",
		"/api/denma/centers", "/api/denma/people", "/api/denma/domains", "/api/denma/gmail", "/api/denma/hub", "/api/denma/mcp",
	} {
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}

// placeCenter is the center a request at /admin or /api is for ("": the hub), or the center's own address for a page load.
func (d *denmaCenters) placeCenter(c echo.Context, p string) (string, string) {
	if !denmaAdminRequest(p) || denmaHubOnlyPath(p) {
		return "", ""
	}
	_, err := c.Cookie(denmaHubSessionCookie)
	hub := err == nil
	// The hub's pages' own script and styles (hub settings).
	if hub && (p == uriAdmin+"/custom.css" || p == uriAdmin+"/custom.js") {
		return "", ""
	}
	slug, named := denmaTabCenter(c, p)
	if slug == "" {
		return "", ""
	}
	// The place's center gone: the hub. A page's own center gone: not found (cmd/denma_centers.go).
	if d.get(slug) == nil && !d.isStarting(slug) && !named {
		return "", ""
	}
	if !denmaPageLoad(c, p) {
		return slug, ""
	}
	// A superadmin's hub pages stay the hub's.
	if hub && denmaHubPath(p) {
		return "", ""
	}
	to := denmaCenterPath + slug + p
	if q := c.Request().URL.RawQuery; q != "" {
		to += "?" + q
	}
	return "", to
}

// denmaPageLoad reports whether a request is a browser's load of an admin page.
func denmaPageLoad(c echo.Context, p string) bool {
	req := c.Request()
	return (req.Method == http.MethodGet || req.Method == http.MethodHead) &&
		req.Header.Get("Sec-Fetch-Mode") == "navigate" &&
		(p == uriAdmin || strings.HasPrefix(p, uriAdmin+"/")) && !strings.HasPrefix(p, uriAdmin+"/static/")
}

// denmaTabHeader is the center ("" for the hub) the admin page was loaded for (denma-place.js).
const denmaTabHeader = "X-Denma-Center"

// denmaTabCenter is the center of an API request's page if it says (named), else the place cookie's.
func denmaTabCenter(c echo.Context, p string) (string, bool) {
	if p == "/api" || strings.HasPrefix(p, "/api/") {
		if h, ok := c.Request().Header[denmaTabHeader]; ok && len(h) > 0 {
			return h[0], h[0] != ""
		}
	}
	return denmaPlace(c), false
}

// DenmaToHub is the top bar's way back to the hub, for superadmins in a center.
func (a *App) DenmaToHub(c echo.Context) error {
	return c.Redirect(http.StatusFound, uriAdmin)
}
