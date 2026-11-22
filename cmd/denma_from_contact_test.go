package main

import (
	"testing"

	"github.com/knadh/listmonk/models"
)

// A campaign whose footer shows its From as the center's e-mail passes the
// footer check with that address or the center's; other campaigns need the center's.
func TestDenmaFooterCheckCampaign(t *testing.T) {
	const slug = "check-from-contact"
	denmaSetContact(slug, denmaTestContact)
	footer := func(email string) []byte {
		return []byte(`<p>1084 Tatamagouche Mountain Road<br>Tatamagouche NS B0K 1V0<br>dorjedenmaling.org · <a href="mailto:` + email + `">` + email + `</a></p>`)
	}
	on := &models.Campaign{FromEmail: `"DDL News" <news@dorjedenmaling.org>`, FromAsContact: true}
	off := &models.Campaign{FromEmail: on.FromEmail}
	for _, c := range []struct {
		camp  *models.Campaign
		email string
		ok    bool
	}{
		{on, "news@dorjedenmaling.org", true},
		{on, "programs@dorjedenmaling.org", true},
		{on, "someone@example.org", false},
		{off, "news@dorjedenmaling.org", false},
		{off, "programs@dorjedenmaling.org", true},
		{nil, "programs@dorjedenmaling.org", true},
	} {
		err := denmaFooterCheckCampaign(slug, c.camp, footer(c.email))
		if (err == nil) != c.ok {
			t.Errorf("on %v, %s: %v", c.camp != nil && c.camp.FromAsContact, c.email, err)
		}
		if err := denmaFooterCheckSent(slug, c.camp, footer(c.email)); (err == nil) != c.ok {
			t.Errorf("as sent, on %v, %s: %v", c.camp != nil && c.camp.FromAsContact, c.email, err)
		}
	}
}
