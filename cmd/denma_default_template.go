// denma: a new center's default visual template: its logo, the content, and the footer with its details.
package main

import (
	"encoding/json"
	"html"
	"strconv"

	"github.com/jmoiron/sqlx"
)

const (
	denmaDefaultTemplateName = "Default template"
	// denmaDefaultTemplateLogo is the logo's place in static/email-templates/denma-default-visual.*.
	denmaDefaultTemplateLogo = "https://denma-center-logo.invalid/logo.png"
)

// denmaAddDefaultTemplate makes a new center's visual template its default template (its logo at its first load).
func denmaAddDefaultTemplate(db *sqlx.DB) error {
	body, err := fs.Read("/static/email-templates/denma-default-visual.tpl")
	if err != nil {
		return err
	}
	src, err := fs.Read("/static/email-templates/denma-default-visual.json")
	if err != nil {
		return err
	}
	var id int
	if err := db.Get(&id, `UPDATE templates SET name = $1, body = $2, body_source = $3, updated_at = NOW()
		WHERE id = (SELECT MIN(id) FROM templates WHERE type = 'campaign_visual') RETURNING id`, denmaDefaultTemplateName, string(body), string(src)); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO settings (key, value) VALUES ('denma.visual_template', $1::TEXT::JSONB)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`, strconv.Itoa(id))
	return err
}

// denmaDefaultTemplateLogoIn puts the center's logo, as its design shows it, in a new default template.
func denmaDefaultTemplateLogoIn(tx *sqlx.Tx) error {
	var logo, root string
	if err := tx.QueryRow(`SELECT COALESCE((SELECT value #>> '{}' FROM settings WHERE key = 'app.logo_url'), ''),
		COALESCE((SELECT value #>> '{}' FROM settings WHERE key = 'app.root_url'), '')`).Scan(&logo, &root); err != nil {
		return err
	}
	_, err := tx.Exec(denmaReplaceLogoSQL+` WHERE type = 'campaign_visual' AND strpos(body_source, $3) > 0`,
		denmaLogoArgs(denmaDefaultTemplateLogo, denmaDesignLogo(logo, root))...)
	return err
}

// denmaReplaceLogoSQL puts a logo URL in place of another in templates' HTML and source (denmaLogoArgs).
const denmaReplaceLogoSQL = `UPDATE templates SET body = replace(body, $1, $2), body_source = replace(body_source, $3, $4), updated_at = NOW()`

// denmaLogoArgs are denmaReplaceLogoSQL's arguments: from and to, as HTML and as JSON.
func denmaLogoArgs(from, to string) []any {
	fromJS, _ := json.Marshal(from)
	toJS, _ := json.Marshal(to)
	return []any{html.EscapeString(from), html.EscapeString(to), string(fromJS), string(toJS)}
}
