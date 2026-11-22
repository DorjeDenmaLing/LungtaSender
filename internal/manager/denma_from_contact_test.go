package manager

import (
	"html/template"
	"testing"

	"github.com/knadh/listmonk/models"
)

// {{ CenterEmail }} is the campaign's From address when it says so, a link and as text.
func TestDenmaFromContactFuncs(t *testing.T) {
	center := func() template.FuncMap {
		return template.FuncMap{"CenterEmail": func() template.HTML { return "center" }, "CenterEmailText": func() string { return "center" }}
	}
	for _, c := range []struct {
		camp       *models.Campaign
		html, text string
	}{
		{&models.Campaign{FromEmail: `"A & B" <news@example.org>`, FromAsContact: true},
			`<a href="mailto:news@example.org" style="color:inherit;">news@example.org</a>`, "news@example.org"},
		{&models.Campaign{FromEmail: "news@example.org"}, "center", "center"},
		{&models.Campaign{FromEmail: "not an address", FromAsContact: true}, "center", "center"},
		{nil, "center", "center"},
	} {
		f := center()
		denmaFromContactFuncs(f, c.camp)
		html := f["CenterEmail"].(func() template.HTML)()
		text := f["CenterEmailText"].(func() string)()
		if string(html) != c.html || text != c.text {
			t.Errorf("%+v: %q, %q", c.camp, html, text)
		}
	}
}
