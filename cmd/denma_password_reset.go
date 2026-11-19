package main

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// denmaResetsByCenter limits the reset links a center's admins send in a day.
var denmaResetsByCenter = newDenmaThrottle(20, 24*time.Hour)

const denmaResetsAddressMsg = "They've been sent 3 links to set a password in the last hour. Try again later."

func initDenmaPasswordResetHandlers(g *echo.Group, a *App) {
	g.POST("/api/users/:id/password-reset", a.auth.Perm(hasID(a.DenmaUserPasswordReset), "users:manage"))
	g.POST("/api/denma/people/:id/password-reset", hasID(a.DenmaPersonPasswordReset))
}

// DenmaUserPasswordReset e-mails one of the center's people a link to set a new password, from this center.
func (a *App) DenmaUserPasswordReset(c echo.Context) error {
	if !a.denmaInCenter() {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	p, err := a.denmaMemberOf(getID(c))
	if err != nil {
		return err
	}
	if p == nil {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	slug := a.ko.String("denma.center")
	if denmaResetsByCenter.wait(slug) > 0 {
		return echo.NewHTTPError(http.StatusTooManyRequests, "The center's admins have sent 20 links to set a password today. Try again tomorrow.")
	}
	if !denmaResetAllowed(p.Email) {
		return echo.NewHTTPError(http.StatusTooManyRequests, denmaResetsAddressMsg)
	}
	denmaResetsByCenter.add(slug)
	go a.denmaSendReset(p, a)
	return c.JSON(http.StatusOK, okResp{true})
}

// DenmaPersonPasswordReset e-mails a person a link to set a new password, for the hub's People page.
func (a *App) DenmaPersonPasswordReset(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	p, err := d.person(getID(c))
	if err != nil {
		return denmaInternal(err)
	}
	if p == nil {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if !denmaResetAllowed(p.Email) {
		return echo.NewHTTPError(http.StatusTooManyRequests, denmaResetsAddressMsg)
	}
	go a.denmaSendReset(p, nil)
	return c.JSON(http.StatusOK, okResp{true})
}

// denmaResetAllowed counts a reset link to email with the forgotten-password ones (cmd/denma_throttle.go).
func denmaResetAllowed(email string) bool {
	key := denmaAccountKey(email)
	if denmaForgotByEmail.wait(key) > 0 {
		return false
	}
	denmaForgotByEmail.add(key)
	return true
}
