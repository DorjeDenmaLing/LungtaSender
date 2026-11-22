package main

// denma: a campaign whose footer shows its From address as the center's e-mail ({{ CenterEmail }}).

import (
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

// denmaFromContactSQL adds whether a campaign's footer shows its From as the center's e-mail.
const denmaFromContactSQL = `ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS denma_from_as_contact BOOLEAN NOT NULL DEFAULT false;`

// denmaCampaignContacts is the details a campaign's e-mail may show besides the center's: its From as the e-mail.
func denmaCampaignContacts(slug string, camp *models.Campaign) []denmaContact {
	if camp == nil || !camp.FromAsContact {
		return nil
	}
	addr := models.DenmaFromAddress(camp.FromEmail)
	if addr == "" {
		return nil
	}
	c := denmaContactOf(slug)
	c.Email = addr
	return []denmaContact{c}
}

// denmaFooterCheckCampaign refuses a campaign's e-mail, as rendered, that lacks the center's details.
func denmaFooterCheckCampaign(slug string, camp *models.Campaign, body []byte) error {
	return denmaFooterCheckWith(slug, body, denmaCampaignContacts(slug, camp))
}

// denmaCheckCampaignMessage is denmaCheckMessage for a campaign's e-mail.
func (a *App) denmaCheckCampaignMessage(camp *models.Campaign, body []byte) error {
	if a.ko.Bool("denma.multi_center") {
		if err := denmaFooterCheckCampaign(a.ko.String("denma.center"), camp, body); err != nil {
			return err
		}
	}
	return a.denmaCheckImages(body)
}

// denmaPreviewFrom gives a preview the editor's unsaved From and its switch.
func denmaPreviewFrom(c echo.Context, camp *models.Campaign) {
	if v, ok := c.Request().Form["from_as_contact"]; ok && len(v) > 0 {
		camp.FromAsContact = v[0] == "true"
		camp.FromEmail = c.FormValue("from_email")
	}
}
