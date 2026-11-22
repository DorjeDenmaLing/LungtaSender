package manager

// denma: a campaign whose footer shows its From address as the center's e-mail (cmd/denma_from_contact.go).

import (
	"html"
	"html/template"

	"github.com/knadh/listmonk/models"
)

// denmaFromContactFuncs makes {{ CenterEmail }} the campaign's From address, when it says so.
func denmaFromContactFuncs(f template.FuncMap, c *models.Campaign) {
	if c == nil || !c.FromAsContact {
		return
	}
	addr := models.DenmaFromAddress(c.FromEmail)
	if addr == "" {
		return
	}
	f["CenterEmail"] = func() template.HTML {
		return template.HTML(`<a href="mailto:` + html.EscapeString(addr) + `" style="color:inherit;">` + html.EscapeString(addr) + `</a>`)
	}
	f["CenterEmailText"] = func() string { return addr }
}
