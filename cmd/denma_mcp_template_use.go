package main

// denma: whether a template is used by scheduled or running campaigns or active automations, for the MCP's edit check.

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func initDenmaTemplateUseHandlers(g *echo.Group, a *App) {
	g.GET("/api/templates/:id/uses", a.auth.Perm(hasID(a.DenmaTemplateUses), "templates:get"))
}

// denmaTemplateUse is how many e-mails on their way or switched on use a template.
type denmaTemplateUse struct {
	Scheduled   int  `db:"scheduled" json:"scheduled"`     // campaigns
	Running     int  `db:"running" json:"running"`         // campaigns
	Automations int  `db:"automations" json:"automations"` // that are on
	InUse       bool `db:"-" json:"in_use"`
}

// DenmaTemplateUses answers how many scheduled or running campaigns and active automations use a template.
func (a *App) DenmaTemplateUses(c echo.Context) error {
	id := getID(c)
	var found bool
	if err := a.db.Get(&found, `SELECT EXISTS (SELECT 1 FROM templates WHERE id = $1)`, id); err != nil {
		return denmaInternal(err)
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "That template doesn't exist.")
	}
	var u denmaTemplateUse
	if err := a.db.Get(&u, `SELECT
		(SELECT COUNT(*) FROM campaigns WHERE template_id = $1 AND status = 'scheduled') AS scheduled,
		(SELECT COUNT(*) FROM campaigns WHERE template_id = $1 AND status = 'running') AS running,
		(SELECT COUNT(*) FROM denma_automations a WHERE a.active AND (a.template_id = $1 OR EXISTS (
			SELECT 1 FROM denma_automation_steps st WHERE st.automation_id = a.id AND st.template_id = $1))) AS automations`, id); err != nil {
		return denmaInternal(err)
	}
	u.InUse = u.Scheduled+u.Running+u.Automations > 0
	return c.JSON(http.StatusOK, okResp{u})
}
