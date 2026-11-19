package main

// denma: Server webhooks' notices: unsubscribes, complaints, bounces and blocklistings of the people each signed up.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/listmonk/internal/utils"
	"github.com/labstack/echo/v4"
)

// denmaSignupNotifySQL adds the notices' columns and contacts, and shared webhooks' rows (denmaFeaturesSQL).
const denmaSignupNotifySQL = `
-- A shared Server webhook turned on here (cmd/denma_hubsignup.go).
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS hub_hook_id INTEGER NULL;
CREATE UNIQUE INDEX IF NOT EXISTS denma_signup_hooks_hub ON denma_signup_hooks (hub_hook_id) WHERE hub_hook_id IS NOT NULL;
-- A Server webhook's notices: where, signed with what, since when, and how far they've gone.
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS notify_url TEXT NOT NULL DEFAULT '';
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS notify_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS notify_since TIMESTAMP WITH TIME ZONE NULL;
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS notify_after BIGINT NOT NULL DEFAULT 0;
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS notify_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS notify_next_at TIMESTAMP WITH TIME ZONE NULL;
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS notify_error TEXT NOT NULL DEFAULT '';
ALTER TABLE denma_signup_hooks ADD COLUMN IF NOT EXISTS notify_ok_at TIMESTAMP WITH TIME ZONE NULL;
-- The people each Server webhook signed up, by address hash.
CREATE TABLE IF NOT EXISTS denma_signup_contacts (
    hook_id    INTEGER NOT NULL REFERENCES denma_signup_hooks(id) ON DELETE CASCADE,
    email_hash TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (hook_id, email_hash)
);
`

// denmaSignupEventTypes are the events a Server webhook is told of.
var denmaSignupEventTypes = []string{"unsubscribed", "complained", "bounced", "blocklisted"}

// denmaSignupEventsSQL is a Server webhook's events after a place in the feed ($1), at most $2, since $4 if given.
const denmaSignupEventsSQL = `SELECT e.seq AS id, e.type, e.email, e.reason, NULL::INT AS campaign_id, e.created_at
	FROM denma_api_events e
	WHERE e.seq > $1 AND e.key_id IS NULL AND e.email IS NOT NULL
	AND e.type IN ('unsubscribed', 'complained', 'bounced', 'blocklisted')
	AND ($4::TIMESTAMPTZ IS NULL OR e.created_at >= $4)
	AND EXISTS (SELECT 1 FROM denma_signup_contacts k
		WHERE k.hook_id = $3 AND k.email_hash = denma_email_hash(e.email) AND k.created_at <= e.created_at)
	ORDER BY e.seq LIMIT $2`

// signupContact records that a Server webhook signed someone up, for its notices.
func (a *App) signupContact(hookID int, email string) {
	if _, err := a.db.Exec(`INSERT INTO denma_signup_contacts (hook_id, email_hash) VALUES ($1, denma_email_hash($2))
		ON CONFLICT DO NOTHING`, hookID, email); err != nil {
		a.log.Printf("denma: error recording sign-up webhook %d's contact: %v", hookID, err)
	}
}

// denmaCheckNotifyURL checks a webhook's address: https only (http too with denmaWebhookPrivate), no user name.
func denmaCheckNotifyURL(a *App, s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || len(s) > 2000 || (u.Scheme != "https" && !(u.Scheme == "http" && denmaWebhookPrivate(a))) {
		return errors.New("The webhook needs a full https:// address, so that the addresses in its events are encrypted on the way.")
	}
	if u.User != nil {
		return errors.New("Put no user name or password in the webhook's address; check its signature instead.")
	}
	return nil
}

// denmaNewNotifySecret makes a notices' signing secret.
func denmaNewNotifySecret() (string, error) {
	r, err := utils.GenerateRandomString(32)
	if err != nil {
		return "", err
	}
	return "whsec_" + r, nil
}

// denmaSignupNotices is what a Server webhook's notices are sent as.
type denmaSignupNotices struct {
	Center     string          `json:"center"`
	CenterSlug string          `json:"center_slug"`
	Webhook    string          `json:"webhook"`
	Events     []denmaAPIEvent `json:"events"`
}

// postSignupNotices sends a Server webhook's notices, returning why it failed.
func (a *App) postSignupNotices(to, secret, hook string, events []denmaAPIEvent) error {
	slug := a.ko.String("denma.center")
	body, err := json.Marshal(denmaSignupNotices{Center: a.ko.String("app.site_name"), CenterSlug: slug, Webhook: hook, Events: events})
	if err != nil {
		return err
	}
	return a.postWebhookBody(to, secret, body)
}

// postWebhookBody posts a signed body to a webhook, returning why it failed.
func (a *App) postWebhookBody(to, secret string, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), denmaAPIWebhookTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, to, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "listmonk-webhooks/1.0")
	req.Header.Set("X-Listmonk-Signature", denmaWebhookSignature(secret, time.Now().Unix(), body))
	resp, err := denmaWebhookClient(denmaWebhookPrivate(a)).Do(req)
	if errors.Is(err, errDenmaPrivateAddr) {
		return errDenmaPrivateAddr
	}
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			return ue.Err
		}
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("it answered %s", resp.Status)
	}
	return nil
}

// denmaSignupNotifyTarget is a Server webhook whose notices are due: its own address, or its shared webhook's.
type denmaSignupNotifyTarget struct {
	ID       int          `db:"id"`
	Name     string       `db:"name"`
	URL      string       `db:"url"`
	Secret   string       `db:"secret"`
	Since    sql.NullTime `db:"since"`
	After    int64        `db:"notify_after"`
	Failures int          `db:"notify_failures"`
}

// signupSendNotices sends each Server webhook its new events, from the API's minute job.
func (a *App) signupSendNotices() {
	var has bool
	if err := a.db.Get(&has, `SELECT EXISTS (SELECT 1 FROM denma_signup_contacts)`); err != nil || !has {
		return
	}
	if err := a.apiPublishEvents(); err != nil {
		a.log.Printf("denma: error placing the events: %v", err)
		return
	}
	if _, err := a.db.Exec(`DELETE FROM denma_api_events WHERE created_at < NOW() - make_interval(days => $1)`, denmaAPIEventDays); err != nil {
		a.log.Printf("denma: error deleting old events: %v", err)
	}
	var hooks []denmaSignupNotifyTarget
	if err := a.db.Select(&hooks, `SELECT h.id, COALESCE(hh.name, h.name) AS name,
		CASE WHEN h.hub_hook_id IS NULL THEN h.notify_url ELSE hh.notify_url END AS url,
		CASE WHEN h.hub_hook_id IS NULL THEN h.notify_secret ELSE hh.notify_secret END AS secret,
		CASE WHEN h.hub_hook_id IS NULL THEN h.notify_since ELSE hh.notify_since END AS since,
		h.notify_after, h.notify_failures
		FROM denma_signup_hooks h LEFT JOIN denma.hub_signup_hooks hh ON hh.id = h.hub_hook_id
		WHERE h.kind = 'server' AND (h.hub_hook_id IS NULL OR hh.id IS NOT NULL)
		AND (h.notify_next_at IS NULL OR h.notify_next_at <= NOW())
		AND EXISTS (SELECT 1 FROM denma_api_events e WHERE e.seq > h.notify_after)`); err != nil {
		a.log.Printf("denma: error reading the sign-up webhooks' notices: %v", err)
		return
	}
	for _, h := range hooks {
		if h.URL == "" {
			continue
		}
		var top int64
		if err := a.db.Get(&top, `SELECT COALESCE(MAX(seq), 0) FROM denma_api_events`); err != nil {
			a.log.Printf("denma: error reading the events: %v", err)
			return
		}
		var since any
		if h.Since.Valid {
			since = h.Since.Time
		}
		for range denmaAPIWebhookPosts {
			var events []denmaAPIEvent
			if err := a.db.Select(&events, denmaSignupEventsSQL, h.After, denmaAPIWebhookBatch, h.ID, since); err != nil {
				a.log.Printf("denma: error reading sign-up webhook %d's events: %v", h.ID, err)
				return
			}
			if len(events) == 0 {
				if _, err := a.db.Exec(`UPDATE denma_signup_hooks SET notify_after = GREATEST(notify_after, $3)
					WHERE id = $1 AND notify_after = $2`, h.ID, h.After, top); err != nil {
					a.log.Printf("denma: error recording sign-up webhook %d's notices: %v", h.ID, err)
				}
				break
			}
			if err := a.postSignupNotices(h.URL, h.Secret, h.Name, events); err != nil {
				h.Failures++
				a.log.Printf("denma: sign-up webhook %d's notices failed (%d in a row): %v", h.ID, h.Failures, err)
				if _, err2 := a.db.Exec(`UPDATE denma_signup_hooks SET notify_failures = $2, notify_error = $3, notify_next_at = $4 WHERE id = $1`,
					h.ID, h.Failures, err.Error(), time.Now().Add(denmaWebhookWait(h.Failures))); err2 != nil {
					a.log.Printf("denma: error recording sign-up webhook %d's failure: %v", h.ID, err2)
				}
				break
			}
			h.After, h.Failures = events[len(events)-1].ID, 0
			if _, err := a.db.Exec(`UPDATE denma_signup_hooks SET notify_after = $2, notify_failures = 0, notify_error = '',
				notify_next_at = NULL, notify_ok_at = NOW() WHERE id = $1`, h.ID, h.After); err != nil {
				a.log.Printf("denma: error recording sign-up webhook %d's notices: %v", h.ID, err)
				break
			}
		}
	}
}

// DenmaSignupServerEvents lists a Server webhook's events after an ID (0 for the oldest kept), oldest first.
func (a *App) DenmaSignupServerEvents(c echo.Context) error {
	h, err := a.signupServerAuth(c, false)
	if err != nil {
		return err
	}
	after, _ := strconv.ParseInt(c.QueryParam("after"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 100
	}
	limit = min(limit, 1000)
	if err := a.apiPublishEvents(); err != nil {
		return denmaInternal(err)
	}
	out := []denmaAPIEvent{}
	if err := a.db.Select(&out, denmaSignupEventsSQL, after, limit, h.ID, nil); err != nil {
		return denmaInternal(err)
	}
	next := after
	if len(out) > 0 {
		next = out[len(out)-1].ID
	}
	return c.JSON(http.StatusOK, okResp{map[string]any{"events": out, "after": next, "more": len(out) == limit}})
}

// denmaSignupServerInfo is what GET /signup/server says of the webhook in this center.
type denmaSignupServerInfo struct {
	Name        string        `json:"name"`
	Center      string        `json:"center"`
	CenterSlug  string        `json:"center_slug"`
	Lists       []denmaOption `json:"lists"`
	Tags        []string      `json:"tags"`
	RequestTags []string      `json:"request_tags"`
	CreateTags  bool          `json:"create_tags"`
	Notices     bool          `json:"notices"`
}

// DenmaSignupServerInfo says which lists and tags a Server webhook has in the center.
func (a *App) DenmaSignupServerInfo(c echo.Context) error {
	h, err := a.signupServerAuth(c, false)
	if err != nil {
		return err
	}
	out := denmaSignupServerInfo{Name: h.Name, Center: a.ko.String("app.site_name"), CenterSlug: a.ko.String("denma.center"),
		Lists: []denmaOption{}, Tags: h.Tags, RequestTags: h.RequestTags, CreateTags: h.CreateTags, Notices: h.NotifyURL != "" || h.Notices}
	if err := a.db.Select(&out.Lists, `SELECT id, name, optin::TEXT AS optin FROM lists WHERE id = ANY($1) ORDER BY id`, h.ListIDs); err != nil {
		return denmaInternal(err)
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// DenmaNewSignupNotifySecret replaces a center Server webhook's signing secret, shown once.
func (a *App) DenmaNewSignupNotifySecret(c echo.Context) error {
	if err := a.inCenter(); err != nil {
		return err
	}
	h, err := a.signupHookToChange(c)
	if err != nil {
		return err
	}
	if h.HubHookID != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Its notices are the shared webhook's: a superadmin changes them on the hub's Webhooks page.")
	}
	if h.Kind != denmaSignupServer || h.NotifyURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "The webhook sends no notices.")
	}
	secret, err := denmaNewNotifySecret()
	if err != nil {
		return denmaInternal(err)
	}
	if _, err := a.db.Exec(`UPDATE denma_signup_hooks SET notify_secret = $2, updated_at = NOW() WHERE id = $1`, h.ID, secret); err != nil {
		return denmaInternal(err)
	}
	if h, err = a.signupHook(h.ID); err != nil {
		return err
	}
	h.NotifySecret = secret
	return c.JSON(http.StatusOK, okResp{h})
}

// DenmaTestSignupNotices sends a center Server webhook's notices address a test event, now.
func (a *App) DenmaTestSignupNotices(c echo.Context) error {
	if err := a.inCenter(); err != nil {
		return err
	}
	h, err := a.signupHookToChange(c)
	if err != nil {
		return err
	}
	if h.HubHookID != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Its notices are the shared webhook's: a superadmin tests them on the hub's Webhooks page.")
	}
	var url, secret string
	if err := a.db.QueryRow(`SELECT notify_url, notify_secret FROM denma_signup_hooks WHERE id = $1`, h.ID).Scan(&url, &secret); err != nil {
		return denmaInternal(err)
	}
	if url == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "The webhook sends no notices.")
	}
	if err := a.postSignupNotices(url, secret, h.Name, []denmaAPIEvent{denmaSignupTestEvent()}); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "The address didn't take it: "+err.Error())
	}
	return c.JSON(http.StatusOK, okResp{true})
}

// denmaSignupTestEvent is the test event the Webhooks pages send.
func denmaSignupTestEvent() denmaAPIEvent {
	return denmaAPIEvent{Type: "test", Email: "test@example.org", Reason: "a test from the Webhooks page", CreatedAt: time.Now()}
}

// signupNotifyForm checks a center Server webhook's notices address, returning a new secret if it's new.
func (a *App) signupNotifyForm(old denmaSignupHook, f *denmaSignupHookForm) (string, error) {
	f.NotifyURL = strings.TrimSpace(f.NotifyURL)
	if f.Kind != denmaSignupServer || old.HubHookID != nil {
		f.NotifyURL = ""
		return "", nil
	}
	if f.NotifyURL == "" {
		return "", nil
	}
	if err := denmaCheckNotifyURL(a, f.NotifyURL); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if old.NotifyURL != "" {
		return "", nil
	}
	secret, err := denmaNewNotifySecret()
	if err != nil {
		return "", denmaInternal(err)
	}
	return secret, nil
}
