package main

// denma: shared Server webhooks, made by superadmins in the hub; each center turns one on with its own lists.

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const denmaHubSignupPerMinute = 1200 // a shared webhook's requests a minute, all centers

// denmaHubSignupSQL makes the shared webhooks' tables (denmaInitRegistry).
const denmaHubSignupSQL = `
CREATE TABLE IF NOT EXISTS denma.hub_signup_hooks (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    token_hash      TEXT NOT NULL UNIQUE, -- SHA-256 of its secret, hex
    token_hint      TEXT NOT NULL,
    old_token_hash  TEXT NULL,
    old_token_until TIMESTAMP WITH TIME ZONE NULL,
    create_tags     BOOLEAN NOT NULL DEFAULT FALSE,
    allowed_from    TEXT[] NOT NULL DEFAULT '{}',
    notify_url      TEXT NOT NULL DEFAULT '',
    notify_secret   TEXT NOT NULL DEFAULT '',
    notify_since    TIMESTAMP WITH TIME ZONE NULL,
    created_by      TEXT NOT NULL DEFAULT '',
    uses            INTEGER NOT NULL DEFAULT 0,
    last_used_at    TIMESTAMP WITH TIME ZONE NULL,
    last_ip         TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS hub_signup_hooks_old_hash ON denma.hub_signup_hooks (old_token_hash) WHERE old_token_hash IS NOT NULL;
-- The centers each is on in.
CREATE TABLE IF NOT EXISTS denma.hub_signup_centers (
    hook_id   INTEGER NOT NULL REFERENCES denma.hub_signup_hooks(id) ON DELETE CASCADE,
    center_id INTEGER NOT NULL REFERENCES denma.centers(id) ON DELETE CASCADE,
    on_at     TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (hook_id, center_id)
);`

func denmaInitHubSignup(db *sqlx.DB) error {
	_, err := db.Exec(denmaHubSignupSQL)
	return err
}

// denmaHubSignupHook is a shared Server webhook, for the hub's Webhooks page and a center's.
type denmaHubSignupHook struct {
	ID           int            `db:"id" json:"id"`
	Name         string         `db:"name" json:"name"`
	TokenHint    string         `db:"token_hint" json:"token_hint"`
	OldUntil     *time.Time     `db:"old_token_until" json:"old_token_until"`
	CreateTags   bool           `db:"create_tags" json:"create_tags"`
	AllowedFrom  pq.StringArray `db:"allowed_from" json:"allowed_from"`
	NotifyURL    string         `db:"notify_url" json:"notify_url"`
	NotifySecret string         `db:"notify_secret" json:"-"`
	CreatedBy    string         `db:"created_by" json:"created_by"`
	Uses         int            `db:"uses" json:"uses"`
	LastUsedAt   *time.Time     `db:"last_used_at" json:"last_used_at"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`

	Centers []denmaHubSignupCenter `db:"-" json:"centers"`

	// Shown once, when made or replaced.
	Secret          string `db:"-" json:"secret,omitempty"`
	NewNotifySecret string `db:"-" json:"notify_secret,omitempty"`
}

type denmaHubSignupCenter struct {
	Slug string    `db:"slug" json:"slug"`
	Name string    `db:"name" json:"name"`
	OnAt time.Time `db:"on_at" json:"on_at"`
}

const denmaHubSignupHookSQL = `SELECT id, name, token_hint, CASE WHEN old_token_until > NOW() THEN old_token_until END AS old_token_until,
	create_tags, allowed_from, notify_url, notify_secret, created_by, uses, last_used_at, created_at FROM denma.hub_signup_hooks`

func initDenmaHubSignupHandlers(g *echo.Group, a *App) {
	g.GET(path.Join(uriAdmin, "/webhooks"), a.ViewDenmaHubWebhooks)
	g.POST("/api/denma/hub/signup-hooks", a.DenmaCreateHubSignupHook)
	g.PUT("/api/denma/hub/signup-hooks/:id", a.DenmaUpdateHubSignupHook)
	g.POST("/api/denma/hub/signup-hooks/:id/key", a.DenmaNewHubSignupKey)
	g.POST("/api/denma/hub/signup-hooks/:id/notify-secret", a.DenmaNewHubNotifySecret)
	g.POST("/api/denma/hub/signup-hooks/:id/notify-test", a.DenmaTestHubNotices)
	g.DELETE("/api/denma/hub/signup-hooks/:id", a.DenmaDeleteHubSignupHook)
}

// hubSignupHooks are the shared webhooks, with the centers each is on in.
func (d *denmaCenters) hubSignupHooks(where string, args ...any) ([]denmaHubSignupHook, error) {
	out := []denmaHubSignupHook{}
	if err := d.base.db.Select(&out, denmaHubSignupHookSQL+where+` ORDER BY name, id`, args...); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Centers = []denmaHubSignupCenter{}
		if err := d.base.db.Select(&out[i].Centers, `SELECT c.slug, c.name, x.on_at FROM denma.hub_signup_centers x
			JOIN denma.centers c ON c.id = x.center_id WHERE x.hook_id = $1 ORDER BY c.name`, out[i].ID); err != nil {
			return nil, err
		}
		if out[i].AllowedFrom == nil {
			out[i].AllowedFrom = pq.StringArray{}
		}
	}
	return out, nil
}

func (d *denmaCenters) hubSignupHook(id int) (denmaHubSignupHook, error) {
	hs, err := d.hubSignupHooks(` WHERE id = $1`, id)
	if err != nil {
		return denmaHubSignupHook{}, denmaInternal(err)
	}
	if len(hs) == 0 {
		return denmaHubSignupHook{}, echo.NewHTTPError(http.StatusNotFound, "That webhook doesn't exist (any more).")
	}
	return hs[0], nil
}

// denmaHubWebhooksView is the hub's Webhooks page.
type denmaHubWebhooksView struct {
	adminView
	Hooks []denmaHubSignupHook
}

// ViewDenmaHubWebhooks renders the hub's Webhooks page, for superadmins.
func (a *App) ViewDenmaHubWebhooks(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	view := denmaHubWebhooksView{adminView: newAdminView(c, "Webhooks", "", "denma.webhooks")}
	if view.Hooks, err = d.hubSignupHooks(""); err != nil {
		return denmaInternal(err)
	}
	return c.Render(http.StatusOK, "admin-denma-hub-webhooks", view)
}

// denmaHubSignupForm is a shared webhook as the hub's Webhooks page sends it.
type denmaHubSignupForm struct {
	Name        string   `json:"name"`
	CreateTags  bool     `json:"create_tags"`
	AllowedFrom []string `json:"allowed_from"`
	NotifyURL   string   `json:"notify_url"`
}

func (a *App) checkHubSignupForm(c echo.Context) (denmaHubSignupForm, error) {
	var f denmaHubSignupForm
	if err := c.Bind(&f); err != nil {
		return f, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if f.Name = strings.TrimSpace(f.Name); !strHasLen(f.Name, 1, 200) {
		return f, echo.NewHTTPError(http.StatusBadRequest, "Give the webhook a name, such as the system it's for.")
	}
	from := []string{}
	for _, s := range f.AllowedFrom {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if err := denmaCheckAllowedFrom(s); err != nil {
			return f, echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		from = append(from, s)
	}
	if len(from) > denmaAPIMaxFrom {
		return f, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Give at most %d addresses.", denmaAPIMaxFrom))
	}
	f.AllowedFrom = from
	if f.NotifyURL = strings.TrimSpace(f.NotifyURL); f.NotifyURL != "" {
		if err := denmaCheckNotifyURL(a, f.NotifyURL); err != nil {
			return f, echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}
	return f, nil
}

// DenmaCreateHubSignupHook makes a shared webhook, showing its secret once.
func (a *App) DenmaCreateHubSignupHook(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	f, err := a.checkHubSignupForm(c)
	if err != nil {
		return err
	}
	tok, hash, hint, err := denmaNewSignupToken(denmaSignupServer)
	if err != nil {
		return denmaInternal(err)
	}
	notifySecret := ""
	if f.NotifyURL != "" {
		if notifySecret, err = denmaNewNotifySecret(); err != nil {
			return denmaInternal(err)
		}
	}
	var id int
	if err := d.base.db.Get(&id, `INSERT INTO denma.hub_signup_hooks (name, token_hash, token_hint, create_tags, allowed_from,
		notify_url, notify_secret, notify_since, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $6 <> '' THEN NOW() END, $8) RETURNING id`,
		f.Name, hash, hint, f.CreateTags, pq.Array(f.AllowedFrom), f.NotifyURL, notifySecret, denmaAPIWho(c)); err != nil {
		return denmaInternal(err)
	}
	h, err := d.hubSignupHook(id)
	if err != nil {
		return err
	}
	h.Secret, h.NewNotifySecret = tok, notifySecret
	return c.JSON(http.StatusOK, okResp{h})
}

// DenmaUpdateHubSignupHook changes a shared webhook's name and settings.
func (a *App) DenmaUpdateHubSignupHook(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	id, _ := strconv.Atoi(c.Param("id"))
	old, err := d.hubSignupHook(id)
	if err != nil {
		return err
	}
	f, err := a.checkHubSignupForm(c)
	if err != nil {
		return err
	}
	notifySecret := ""
	if old.NotifyURL == "" && f.NotifyURL != "" {
		if notifySecret, err = denmaNewNotifySecret(); err != nil {
			return denmaInternal(err)
		}
	}
	// A new notices address starts from now.
	if _, err := d.base.db.Exec(`UPDATE denma.hub_signup_hooks SET name = $2, create_tags = $3, allowed_from = $4, notify_url = $5,
		notify_secret = CASE WHEN $6 <> '' THEN $6 ELSE notify_secret END,
		notify_since = CASE WHEN $5 = '' THEN NULL WHEN notify_url IS DISTINCT FROM $5 THEN NOW() ELSE notify_since END,
		updated_at = NOW() WHERE id = $1`, id, f.Name, f.CreateTags, pq.Array(f.AllowedFrom), f.NotifyURL, notifySecret); err != nil {
		return denmaInternal(err)
	}
	if f.NotifyURL != old.NotifyURL {
		if err := d.hubSignupEachCenter(id, `UPDATE %s.denma_signup_hooks SET notify_failures = 0, notify_next_at = NULL, notify_error = ''
			WHERE hub_hook_id = $1`); err != nil {
			return denmaInternal(err)
		}
	}
	h, err := d.hubSignupHook(id)
	if err != nil {
		return err
	}
	h.NewNotifySecret = notifySecret
	return c.JSON(http.StatusOK, okResp{h})
}

// hubSignupEachCenter runs q (its schema as %s, the webhook as $1) in each center the shared webhook is on in.
func (d *denmaCenters) hubSignupEachCenter(id int, q string) error {
	var schemas []string
	if err := d.base.db.Select(&schemas, `SELECT c.schema_name FROM denma.hub_signup_centers x
		JOIN denma.centers c ON c.id = x.center_id WHERE x.hook_id = $1`, id); err != nil {
		return err
	}
	for _, s := range schemas {
		if _, err := d.base.db.Exec(fmt.Sprintf(q, pq.QuoteIdentifier(s)), id); err != nil {
			return err
		}
	}
	return nil
}

// DenmaNewHubSignupKey gives a shared webhook a new secret, the old one working a while if asked.
func (a *App) DenmaNewHubSignupKey(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	id, _ := strconv.Atoi(c.Param("id"))
	if _, err := d.hubSignupHook(id); err != nil {
		return err
	}
	var r struct {
		OverlapHours int `json:"overlap_hours"`
	}
	_ = c.Bind(&r)
	if r.OverlapHours < 0 || r.OverlapHours > denmaAPIMaxOverlapHours {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("The old secret can keep working for at most %d hours.", denmaAPIMaxOverlapHours))
	}
	tok, hash, hint, err := denmaNewSignupToken(denmaSignupServer)
	if err != nil {
		return denmaInternal(err)
	}
	if _, err := d.base.db.Exec(`UPDATE denma.hub_signup_hooks SET
		old_token_hash = CASE WHEN $4 > 0 THEN token_hash END,
		old_token_until = CASE WHEN $4 > 0 THEN NOW() + make_interval(hours => $4) END,
		token_hash = $2, token_hint = $3, updated_at = NOW() WHERE id = $1`, id, hash, hint, r.OverlapHours); err != nil {
		return denmaInternal(err)
	}
	h, err := d.hubSignupHook(id)
	if err != nil {
		return err
	}
	h.Secret = tok
	return c.JSON(http.StatusOK, okResp{h})
}

// DenmaNewHubNotifySecret replaces a shared webhook's signing secret, shown once.
func (a *App) DenmaNewHubNotifySecret(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	id, _ := strconv.Atoi(c.Param("id"))
	old, err := d.hubSignupHook(id)
	if err != nil {
		return err
	}
	if old.NotifyURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "The webhook sends no notices.")
	}
	secret, err := denmaNewNotifySecret()
	if err != nil {
		return denmaInternal(err)
	}
	if _, err := d.base.db.Exec(`UPDATE denma.hub_signup_hooks SET notify_secret = $2, updated_at = NOW() WHERE id = $1`, id, secret); err != nil {
		return denmaInternal(err)
	}
	h, err := d.hubSignupHook(id)
	if err != nil {
		return err
	}
	h.NewNotifySecret = secret
	return c.JSON(http.StatusOK, okResp{h})
}

// DenmaTestHubNotices sends a shared webhook's notices address a test event, now.
func (a *App) DenmaTestHubNotices(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	id, _ := strconv.Atoi(c.Param("id"))
	h, err := d.hubSignupHook(id)
	if err != nil {
		return err
	}
	if h.NotifyURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "The webhook sends no notices.")
	}
	if err := a.postSignupNotices(h.NotifyURL, h.NotifySecret, h.Name, []denmaAPIEvent{denmaSignupTestEvent()}); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "The address didn't take it: "+err.Error())
	}
	return c.JSON(http.StatusOK, okResp{true})
}

// DenmaDeleteHubSignupHook deletes a shared webhook, and turns it off in every center.
func (a *App) DenmaDeleteHubSignupHook(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	id, _ := strconv.Atoi(c.Param("id"))
	// A center's row left by a turn-on meanwhile goes when its Webhooks page is next read (signupHooks).
	if err := d.hubSignupEachCenter(id, `DELETE FROM %s.denma_signup_hooks WHERE hub_hook_id = $1`); err != nil {
		return denmaInternal(err)
	}
	if _, err := d.base.db.Exec(`DELETE FROM denma.hub_signup_hooks WHERE id = $1`, id); err != nil {
		return denmaInternal(err)
	}
	return c.JSON(http.StatusOK, okResp{true})
}

// ---- In a center ----

// signupServerAuth finds the Server webhook a request's secret is for: the center's, or a shared one on here; a sign-up counts as a use.
func (a *App) signupServerAuth(c echo.Context, signup bool) (denmaSignupHook, error) {
	var h denmaSignupHook
	if err := a.inCenter(); err != nil {
		return h, err
	}
	ip := c.RealIP()
	if wait := denmaSignupBadSecrets.wait(ip); wait > 0 {
		return h, denmaTooMany(wait)
	}
	secret, ok := denmaBearer(c.Request().Header.Get(echo.HeaderAuthorization), denmaSignupSecretPrefix)
	if !ok {
		denmaSignupBadSecrets.add(ip)
		c.Response().Header().Set("WWW-Authenticate", `Bearer realm="listmonk"`)
		return h, echo.NewHTTPError(http.StatusUnauthorized, "Send the webhook's secret as Authorization: Bearer <secret>.")
	}
	hash := denmaAPIHash(secret)
	err := a.db.Get(&h, denmaSignupHookSQL+` WHERE h.kind = 'server' AND h.hub_hook_id IS NULL
		AND (h.token_hash = $1 OR (h.old_token_hash = $1 AND h.old_token_until > NOW()))`, hash)
	shared := false
	if errors.Is(err, sql.ErrNoRows) {
		h, shared, err = a.signupSharedHook(hash, ip)
	}
	if errors.Is(err, sql.ErrNoRows) {
		denmaSignupBadSecrets.add(ip)
		return h, echo.NewHTTPError(http.StatusUnauthorized, "This secret isn't in use (any more).")
	}
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return h, err
	}
	if err != nil {
		return h, denmaInternal(err)
	}
	if !denmaFromAllowed(ip, h.AllowedFrom) {
		a.log.Printf("denma: sign-up webhook %d (%s) refused from %s: not one of its addresses", h.ID, h.Name, ip)
		return h, echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("This webhook can't be used from %s.", ip))
	}
	c.Set(denmaSignupHookCtx, h)
	if shared && !denmaHubSignupAllowed(*h.HubHookID) {
		return h, echo.NewHTTPError(http.StatusTooManyRequests, fmt.Sprintf("At most %d requests a minute, all centers together. Try again in a minute.", denmaHubSignupPerMinute))
	}
	if !a.signupAllowed(-h.ID, denmaSignupServerPerMinute, time.Minute) {
		return h, echo.NewHTTPError(http.StatusTooManyRequests, fmt.Sprintf("At most %d requests a minute. Try again in a minute.", denmaSignupServerPerMinute))
	}
	if !signup {
		return h, nil
	}
	if _, err := a.db.Exec(`UPDATE denma_signup_hooks SET uses = uses + 1, last_used_at = NOW(), last_ip = $2 WHERE id = $1`, h.ID, ip); err != nil {
		a.log.Printf("denma: error counting sign-up webhook %d's use: %v", h.ID, err)
	}
	if shared {
		if _, err := a.db.Exec(`UPDATE denma.hub_signup_hooks SET uses = uses + 1, last_used_at = NOW(), last_ip = $2 WHERE id = $1`, *h.HubHookID, ip); err != nil {
			a.log.Printf("denma: error counting shared sign-up webhook %d's use: %v", *h.HubHookID, err)
		}
	}
	return h, nil
}

// signupSharedHook is the center's row for the shared webhook with a secret, with the shared settings.
func (a *App) signupSharedHook(hash, ip string) (denmaSignupHook, bool, error) {
	var h denmaSignupHook
	var hh struct {
		ID          int            `db:"id"`
		AllowedFrom pq.StringArray `db:"allowed_from"`
	}
	if err := a.db.Get(&hh, `SELECT id, allowed_from FROM denma.hub_signup_hooks
		WHERE token_hash = $1 OR (old_token_hash = $1 AND old_token_until > NOW())`, hash); err != nil {
		return h, false, err
	}
	if !denmaFromAllowed(ip, hh.AllowedFrom) {
		a.log.Printf("denma: shared sign-up webhook %d refused from %s: not one of its addresses", hh.ID, ip)
		return h, true, echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("This webhook can't be used from %s.", ip))
	}
	if err := a.db.Get(&h, denmaSignupHookSQL+` WHERE h.hub_hook_id = $1`, hh.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return h, true, echo.NewHTTPError(http.StatusForbidden, "This webhook isn't on in this center: its admins turn it on, choosing its lists, on Config -> Webhooks.")
		}
		return h, true, err
	}
	return h, true, nil
}

// denmaHubSignupCount is each shared webhook's requests this minute.
var denmaHubSignupCount = struct {
	m map[int]*denmaSignupWindow
}{m: map[int]*denmaSignupWindow{}}

// denmaHubSignupAllowed counts a request against its shared webhook's limit, all centers together.
func denmaHubSignupAllowed(id int) bool {
	denmaSignupCount.Lock()
	defer denmaSignupCount.Unlock()
	now := time.Now()
	w := denmaHubSignupCount.m[id]
	if w == nil || now.Sub(w.start) >= time.Minute {
		w = &denmaSignupWindow{start: now}
		denmaHubSignupCount.m[id] = w
	}
	w.n++
	return w.n <= denmaHubSignupPerMinute
}

// signupTurnOnShared turns a shared webhook on in the center, as its row, with the form's lists and tags.
func (a *App) signupTurnOnShared(c echo.Context, hubID int, f denmaSignupHookForm) (int, error) {
	var name string
	if err := a.db.Get(&name, `SELECT name FROM denma.hub_signup_hooks WHERE id = $1`, hubID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, echo.NewHTTPError(http.StatusNotFound, "That shared webhook doesn't exist (any more).")
		}
		return 0, denmaInternal(err)
	}
	centerID, err := a.mcpCenterID()
	if err != nil {
		return 0, err
	}
	tx, err := a.db.Begin()
	if err != nil {
		return 0, denmaInternal(err)
	}
	defer tx.Rollback()
	var id int
	err = tx.QueryRow(`INSERT INTO denma_signup_hooks (name, kind, token_hash, token_hint, tags, request_tags, per_hour, hub_hook_id, created_by)
		VALUES ($1, 'server', $7, '', $3, $4, $5, $2, $6) ON CONFLICT DO NOTHING RETURNING id`,
		name, hubID, pq.Array(f.Tags), pq.Array(f.RequestTags), denmaSignupDefaultPerHour, denmaAPIWho(c), "shared:"+strconv.Itoa(hubID)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, echo.NewHTTPError(http.StatusConflict, "It's on in this center already.")
	}
	if err != nil {
		return 0, denmaInternal(err)
	}
	if err := saveSignupLists(tx, id, f.ListIDs); err != nil {
		return 0, denmaInternal(err)
	}
	if _, err := tx.Exec(`INSERT INTO denma.hub_signup_centers (hook_id, center_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, hubID, centerID); err != nil {
		return 0, denmaInternal(err)
	}
	if err := tx.Commit(); err != nil {
		return 0, denmaInternal(err)
	}
	return id, nil
}

// signupTurnOffShared records that a shared webhook is off in the center.
func (a *App) signupTurnOffShared(hubID int) error {
	_, err := a.db.Exec(`DELETE FROM denma.hub_signup_centers WHERE hook_id = $1
		AND center_id = (SELECT id FROM denma.centers WHERE slug = $2)`, hubID, a.ko.String("denma.center"))
	return err
}

// signupSharedOffered are the shared webhooks not on in the center, for Config -> Webhooks.
func (a *App) signupSharedOffered() ([]denmaSharedOffer, error) {
	out := []denmaSharedOffer{}
	err := a.db.Select(&out, `SELECT id, name, create_tags FROM denma.hub_signup_hooks hh
		WHERE NOT EXISTS (SELECT 1 FROM denma_signup_hooks h WHERE h.hub_hook_id = hh.id) ORDER BY name, id`)
	return out, err
}

// denmaSharedOffer is a shared webhook as a center's admins see it, to turn on.
type denmaSharedOffer struct {
	ID         int    `db:"id" json:"id"`
	Name       string `db:"name" json:"name"`
	CreateTags bool   `db:"create_tags" json:"create_tags"`
}
