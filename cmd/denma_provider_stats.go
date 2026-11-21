package main

// denma: Amazon SES's and Gmail's own figures for each sending domain, by day, for the centers' complaint and Gmail spam rates.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"golang.org/x/oauth2"
)

// denmaProviderSchemaSQL holds the figures and the Gmail connection (in denmaInitDomains).
const denmaProviderSchemaSQL = `
CREATE TABLE IF NOT EXISTS denma.provider_days (
    domain          TEXT NOT NULL,
    day             DATE NOT NULL,             -- SES's UTC day, Gmail's own
    ses_complaints  INTEGER NULL,              -- VDM COMPLAINT
    ses_deliveries  INTEGER NULL,              -- VDM DELIVERY_COMPLAINT: to providers that report complaints to SES
    gmail_spam_rate DOUBLE PRECISION NULL,     -- Postmaster Tools SPAM_RATE, a share
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (domain, day)
);
CREATE TABLE IF NOT EXISTS denma.gmail_postmaster (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    client_id     TEXT NOT NULL DEFAULT '',
    client_secret TEXT NOT NULL DEFAULT '',
    refresh_token TEXT NOT NULL DEFAULT '',
    account       TEXT NOT NULL DEFAULT '',
    state         TEXT NOT NULL DEFAULT '',   -- the sign-in under way
    state_at      TIMESTAMP WITH TIME ZONE NULL,
    connected_at  TIMESTAMP WITH TIME ZONE NULL,
    fetched_at    TIMESTAMP WITH TIME ZONE NULL,
    errors        JSONB NOT NULL DEFAULT '{}' -- the last fetch's, by domain ('' for all)
);
INSERT INTO denma.gmail_postmaster (id) VALUES (1) ON CONFLICT DO NOTHING;`

const (
	denmaProviderBackfill = 90               // days fetched for a domain with none yet
	denmaProviderRecent   = 6                // days fetched again each time, as late figures arrive
	denmaGmailEvery       = 6 * time.Hour    // Gmail's figures come once a day
	denmaGmailStateTTL    = 10 * time.Minute // a sign-in's time to come back
	denmaGmailScope       = "https://www.googleapis.com/auth/postmaster.traffic.readonly"
	denmaProviderTimeout  = 10 * time.Minute // one fetch of every domain
	denmaVDMBatch         = 10               // queries in one BatchGetMetricData call
)

// Google's endpoints (a test's server in tests).
var (
	denmaGmailAPI    = "https://gmailpostmastertools.googleapis.com"
	denmaGoogleOAuth = oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token"}
)

// denmaProviderMu lets one fetch run at a time.
var denmaProviderMu sync.Mutex

// denmaSESDomainDay is one UTC day's VDM figures for a domain.
type denmaSESDomainDay struct {
	Domain     string
	Day        time.Time
	Complaints int
	Deliveries int
}

// DomainDays is VDM's complaints and deliveries to reporting providers for each domain, by UTC day, from and to midnights.
func (s *denmaRealSES) DomainDays(ctx context.Context, domains []string, from, to time.Time) ([]denmaSESDomainDay, error) {
	type ref struct {
		domain     string
		complaints bool
	}
	var (
		qs   []sestypes.BatchGetMetricDataQuery
		refs = map[string]ref{}
	)
	for i, d := range domains {
		for _, m := range []sestypes.Metric{sestypes.MetricComplaint, sestypes.MetricDeliveryComplaint} {
			id := fmt.Sprintf("q%d_%s", i, strings.ToLower(string(m)))
			refs[id] = ref{d, m == sestypes.MetricComplaint}
			qs = append(qs, sestypes.BatchGetMetricDataQuery{
				Id: aws.String(id), Namespace: sestypes.MetricNamespaceVdm, Metric: m,
				Dimensions: map[string]string{string(sestypes.MetricDimensionNameEmailIdentity): d},
				StartDate:  aws.Time(from), EndDate: aws.Time(to),
			})
		}
	}
	days := map[string]*denmaSESDomainDay{}
	for i := 0; i < len(qs); i += denmaVDMBatch {
		out, err := s.v2.BatchGetMetricData(ctx, &sesv2.BatchGetMetricDataInput{Queries: qs[i:min(i+denmaVDMBatch, len(qs))]})
		if err != nil {
			return nil, err
		}
		if len(out.Errors) > 0 {
			e := out.Errors[0]
			return nil, fmt.Errorf("%s: %s", e.Code, aws.ToString(e.Message))
		}
		for _, r := range out.Results {
			rf := refs[aws.ToString(r.Id)]
			for j, t := range r.Timestamps {
				if j >= len(r.Values) {
					break
				}
				day := t.UTC().Truncate(24 * time.Hour)
				k := rf.domain + "|" + day.Format(time.DateOnly)
				if days[k] == nil {
					days[k] = &denmaSESDomainDay{Domain: rf.domain, Day: day}
				}
				if rf.complaints {
					days[k].Complaints = int(r.Values[j])
				} else {
					days[k].Deliveries = int(r.Values[j])
				}
			}
		}
	}
	out := make([]denmaSESDomainDay, 0, len(days))
	for _, d := range days {
		out = append(out, *d)
	}
	return out, nil
}

// DomainDays is the dev SES's (denma.dev_ses_vdm), as a check sets it.
func (s *denmaFakeSES) DomainDays(ctx context.Context, domains []string, from, to time.Time) ([]denmaSESDomainDay, error) {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS denma.dev_ses_vdm (domain TEXT NOT NULL, day DATE NOT NULL,
		complaints INTEGER NOT NULL, deliveries INTEGER NOT NULL, PRIMARY KEY (domain, day))`); err != nil {
		return nil, err
	}
	var rows []struct {
		Domain     string    `db:"domain"`
		Day        time.Time `db:"day"`
		Complaints int       `db:"complaints"`
		Deliveries int       `db:"deliveries"`
	}
	if err := s.db.SelectContext(ctx, &rows, `SELECT domain, day, complaints, deliveries FROM denma.dev_ses_vdm
		WHERE domain = ANY($1) AND day >= $2::DATE AND day < $3::DATE`, pq.Array(domains), from.UTC(), to.UTC()); err != nil {
		return nil, err
	}
	out := make([]denmaSESDomainDay, len(rows))
	for i, r := range rows {
		out[i] = denmaSESDomainDay{Domain: r.Domain, Day: r.Day, Complaints: r.Complaints, Deliveries: r.Deliveries}
	}
	return out, nil
}

// watchProviderStats fetches the figures every hour.
func (d *denmaCenters) watchProviderStats() {
	time.Sleep(time.Minute)
	for {
		d.fetchProviderStats(false)
		time.Sleep(time.Hour)
	}
}

// fetchProviderStats fetches SES's figures, and Gmail's when due (or gmailNow).
func (d *denmaCenters) fetchProviderStats(gmailNow bool) {
	denmaProviderMu.Lock()
	defer denmaProviderMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), denmaProviderTimeout)
	defer cancel()
	s, _ := d.ses()
	if s == nil {
		return
	}
	var doms []string
	if err := d.db().SelectContext(ctx, &doms, `SELECT domain FROM denma.sending_domains
		WHERE COALESCE((state->>'can_send')::BOOLEAN, false) ORDER BY domain`); err != nil {
		lo.Printf("denma: error reading the sending domains for their figures: %v", err)
		return
	}
	if len(doms) == 0 {
		return
	}
	if err := d.fetchSESDays(ctx, s, doms); err != nil {
		lo.Printf("denma: error fetching Amazon SES's figures by domain: %v", err)
	}
	g, err := d.gmailConfig()
	if err != nil {
		lo.Printf("denma: error reading the Gmail Postmaster Tools connection: %v", err)
		return
	}
	if g.RefreshToken != "" && (gmailNow || g.FetchedAt == nil || time.Since(*g.FetchedAt) > denmaGmailEvery) {
		_, fake := s.(*denmaFakeSES)
		d.fetchGmailDays(ctx, g, doms, fake)
	}
}

// denmaProviderFrom is where a domain's fetch starts: a few days back, or the backfill for a domain with none.
func denmaProviderFrom(have map[string]bool, dom string, today time.Time) time.Time {
	if have[dom] {
		return today.AddDate(0, 0, -denmaProviderRecent)
	}
	return today.AddDate(0, 0, -denmaProviderBackfill)
}

// denmaProviderHave is the domains with figures of the column already.
func (d *denmaCenters) providerHave(ctx context.Context, col string) (map[string]bool, error) {
	var doms []string
	if err := d.db().SelectContext(ctx, &doms, `SELECT DISTINCT domain FROM denma.provider_days WHERE `+col+` IS NOT NULL`); err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, x := range doms {
		have[x] = true
	}
	return have, nil
}

func (d *denmaCenters) fetchSESDays(ctx context.Context, s denmaSES, doms []string) error {
	have, err := d.providerHave(ctx, "ses_deliveries")
	if err != nil {
		return err
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	groups := map[time.Time][]string{}
	for _, dom := range doms {
		f := denmaProviderFrom(have, dom, today)
		groups[f] = append(groups[f], dom)
	}
	for from, group := range groups {
		days, err := s.DomainDays(ctx, group, from, today.AddDate(0, 0, 1))
		if err != nil {
			return err
		}
		var (
			ds, ts      []string
			comps, dels []int64
		)
		for _, x := range days {
			ds, ts = append(ds, x.Domain), append(ts, x.Day.Format(time.DateOnly))
			comps, dels = append(comps, int64(x.Complaints)), append(dels, int64(x.Deliveries))
		}
		if _, err := d.db().ExecContext(ctx, `INSERT INTO denma.provider_days (domain, day, ses_complaints, ses_deliveries)
			SELECT * FROM UNNEST($1::TEXT[], $2::DATE[], $3::INT[], $4::INT[])
			ON CONFLICT (domain, day) DO UPDATE SET ses_complaints = EXCLUDED.ses_complaints,
				ses_deliveries = EXCLUDED.ses_deliveries, updated_at = NOW()`,
			pq.Array(ds), pq.Array(ts), pq.Array(comps), pq.Array(dels)); err != nil {
			return err
		}
	}
	return nil
}

// Gmail Postmaster Tools.

// denmaGmailConfig is the hub's connection to Gmail Postmaster Tools.
type denmaGmailConfig struct {
	ClientID     string     `db:"client_id"`
	ClientSecret string     `db:"client_secret"`
	RefreshToken string     `db:"refresh_token"`
	Account      string     `db:"account"`
	State        string     `db:"state"`
	StateAt      *time.Time `db:"state_at"`
	ConnectedAt  *time.Time `db:"connected_at"`
	FetchedAt    *time.Time `db:"fetched_at"`
	Errors       []byte     `db:"errors"`
}

func (d *denmaCenters) gmailConfig() (denmaGmailConfig, error) {
	return denmaGmailConfigFrom(d.db())
}

func denmaGmailConfigFrom(db *sqlx.DB) (denmaGmailConfig, error) {
	var g denmaGmailConfig
	err := db.Get(&g, `SELECT client_id, client_secret, refresh_token, account, state, state_at, connected_at, fetched_at, errors
		FROM denma.gmail_postmaster WHERE id = 1`)
	if errors.Is(err, sql.ErrNoRows) {
		return g, nil
	}
	return g, err
}

func (d *denmaCenters) gmailRedirect() string {
	return strings.TrimSuffix(d.current().urlCfg.RootURL, "/") + path.Join(uriAdmin, "/domains/gmail/callback")
}

func (d *denmaCenters) gmailOAuth(g denmaGmailConfig) *oauth2.Config {
	return &oauth2.Config{ClientID: g.ClientID, ClientSecret: g.ClientSecret, Endpoint: denmaGoogleOAuth,
		RedirectURL: d.gmailRedirect(), Scopes: []string{denmaGmailScope, "openid", "email"}}
}

// denmaGmailDay is one day's Gmail spam rate for a domain.
type denmaGmailDay struct {
	Day  string // YYYY-MM-DD
	Rate float64
}

// fetchGmailDays fetches each domain's spam rate and keeps the errors, by domain.
func (d *denmaCenters) fetchGmailDays(ctx context.Context, g denmaGmailConfig, doms []string, fake bool) {
	have, err := d.providerHave(ctx, "gmail_spam_rate")
	if err != nil {
		lo.Printf("denma: error reading Gmail's figures: %v", err)
		return
	}
	var client *http.Client
	if !fake {
		client = d.gmailOAuth(g).Client(ctx, &oauth2.Token{RefreshToken: g.RefreshToken})
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	errs := map[string]string{}
	for _, dom := range doms {
		from := denmaProviderFrom(have, dom, today)
		var days []denmaGmailDay
		if fake {
			days, err = denmaFakeGmailDays(ctx, d.db(), dom, from, today)
		} else {
			days, err = denmaGmailDays(ctx, client, dom, from, today, g.Account)
		}
		if err != nil {
			var re *oauth2.RetrieveError
			if errors.As(err, &re) {
				errs[""] = "Google refused the connection (" + re.ErrorCode + "): connect again."
				break
			}
			errs[dom] = err.Error()
			continue
		}
		for _, x := range days {
			if _, err := d.db().ExecContext(ctx, `INSERT INTO denma.provider_days (domain, day, gmail_spam_rate) VALUES ($1, $2, $3)
				ON CONFLICT (domain, day) DO UPDATE SET gmail_spam_rate = EXCLUDED.gmail_spam_rate, updated_at = NOW()`,
				dom, x.Day, x.Rate); err != nil {
				errs[dom] = "Its figures couldn't be kept."
				lo.Printf("denma: error keeping Gmail's figures: %v", err)
				break
			}
		}
	}
	b, _ := json.Marshal(errs)
	if _, err := d.db().ExecContext(ctx, `UPDATE denma.gmail_postmaster SET fetched_at = NOW(), errors = $1 WHERE id = 1`, b); err != nil {
		lo.Printf("denma: error noting Gmail's fetch: %v", err)
	}
}

// denmaGmailDays asks Postmaster Tools for a domain's daily spam rate, from and to (inclusive).
func denmaGmailDays(ctx context.Context, client *http.Client, dom string, from, to time.Time, account string) ([]denmaGmailDay, error) {
	date := func(t time.Time) map[string]int {
		return map[string]int{"year": t.Year(), "month": int(t.Month()), "day": t.Day()}
	}
	var (
		out   []denmaGmailDay
		token string
	)
	for page := 0; page < 20; page++ {
		req := map[string]any{
			"metricDefinitions":      []any{map[string]any{"name": "spam", "baseMetric": map[string]string{"standardMetric": "SPAM_RATE"}}},
			"timeQuery":              map[string]any{"dateRanges": map[string]any{"dateRanges": []any{map[string]any{"start": date(from), "end": date(to)}}}},
			"aggregationGranularity": "DAILY",
			"pageSize":               200,
		}
		if token != "" {
			req["pageToken"] = token
		}
		body, _ := json.Marshal(req)
		u := denmaGmailAPI + "/v2/domains/" + url.PathEscape(dom) + "/domainStats:query"
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(r)
		if err != nil {
			var re *oauth2.RetrieveError
			if errors.As(err, &re) {
				return nil, re
			}
			return nil, errors.New("Gmail Postmaster Tools couldn't be reached.")
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
			return nil, fmt.Errorf("Not in Gmail Postmaster Tools for %s: add and verify it there.", denmaOr(account, "the account connected"))
		case resp.StatusCode != http.StatusOK:
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(b, &e)
			return nil, fmt.Errorf("Gmail Postmaster Tools answered %d: %s", resp.StatusCode, denmaClip(e.Error.Message, 200))
		}
		var res struct {
			DomainStats []struct {
				Metric string `json:"metric"`
				Date   *struct {
					Year, Month, Day int
				} `json:"date"`
				Value struct {
					DoubleValue *float64 `json:"doubleValue"`
					FloatValue  *float64 `json:"floatValue"`
				} `json:"value"`
			} `json:"domainStats"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return nil, errors.New("Gmail Postmaster Tools' answer couldn't be read.")
		}
		for _, x := range res.DomainStats {
			v := x.Value.DoubleValue
			if v == nil {
				v = x.Value.FloatValue
			}
			if x.Metric != "spam" || x.Date == nil || v == nil {
				continue
			}
			out = append(out, denmaGmailDay{Day: fmt.Sprintf("%04d-%02d-%02d", x.Date.Year, x.Date.Month, x.Date.Day), Rate: *v})
		}
		if token = res.NextPageToken; token == "" {
			break
		}
	}
	return out, nil
}

// denmaFakeGmailDays is the dev Gmail's (denma.dev_gmail_days), as a check sets it.
func denmaFakeGmailDays(ctx context.Context, db *sqlx.DB, dom string, from, to time.Time) ([]denmaGmailDay, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS denma.dev_gmail_days (domain TEXT NOT NULL, day DATE NOT NULL,
		spam_rate DOUBLE PRECISION NOT NULL, PRIMARY KEY (domain, day))`); err != nil {
		return nil, err
	}
	var out []denmaGmailDay
	err := db.SelectContext(ctx, &out, `SELECT day::TEXT AS "day", spam_rate AS "rate" FROM denma.dev_gmail_days
		WHERE domain = $1 AND day BETWEEN $2::DATE AND $3::DATE`, dom, from, to)
	return out, err
}

func denmaOr(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

func denmaClip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// denmaGmailView is the connection, as the Sending domains page shows it.
type denmaGmailView struct {
	ClientID    string            `json:"client_id"`
	HasSecret   bool              `json:"has_secret"`
	Connected   bool              `json:"connected"`
	Account     string            `json:"account"`
	ConnectedAt *time.Time        `json:"connected_at"`
	FetchedAt   *time.Time        `json:"fetched_at"`
	Errors      map[string]string `json:"errors"`
	Redirect    string            `json:"redirect"`
	Notice      string            `json:"notice"` // after a sign-in
}

func (d *denmaCenters) gmailView(notice string) (denmaGmailView, error) {
	g, err := d.gmailConfig()
	if err != nil {
		return denmaGmailView{}, err
	}
	v := denmaGmailView{ClientID: g.ClientID, HasSecret: g.ClientSecret != "", Connected: g.RefreshToken != "", Account: g.Account,
		ConnectedAt: g.ConnectedAt, FetchedAt: g.FetchedAt, Errors: map[string]string{}, Redirect: d.gmailRedirect()}
	_ = json.Unmarshal(g.Errors, &v.Errors)
	switch notice {
	case "connected":
		v.Notice = "Connected. Gmail's figures are being fetched."
	case "denied":
		v.Notice = "The Google sign-in was cancelled; nothing changed."
	case "failed":
		v.Notice = "Google didn't give access. Check the client ID and secret, then connect again."
	case "expired":
		v.Notice = "That sign-in took too long or was already used. Connect again."
	}
	return v, nil
}

// DenmaGmailSave saves the Google client the connection signs in with.
func (a *App) DenmaGmailSave(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	id, secret := strings.TrimSpace(req.ClientID), strings.TrimSpace(req.ClientSecret)
	if id == "" || len(id) > 300 || len(secret) > 300 {
		return echo.NewHTTPError(http.StatusBadRequest, "Enter the client ID from Google Cloud.")
	}
	g, err := d.gmailConfig()
	if err != nil {
		return denmaInternal(err)
	}
	if secret == "" && (g.ClientSecret == "" || id != g.ClientID) {
		return echo.NewHTTPError(http.StatusBadRequest, "Enter the client secret from Google Cloud.")
	}
	if _, err := d.db().Exec(`UPDATE denma.gmail_postmaster SET client_id = $1, client_secret = COALESCE(NULLIF($2, ''), client_secret),
		refresh_token = CASE WHEN client_id = $1 THEN refresh_token ELSE '' END,
		account = CASE WHEN client_id = $1 THEN account ELSE '' END WHERE id = 1`, id, secret); err != nil {
		return denmaInternal(err)
	}
	return d.gmailResp(c)
}

// DenmaGmailDisconnect forgets the Google account's access.
func (a *App) DenmaGmailDisconnect(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	if _, err := d.db().Exec(`UPDATE denma.gmail_postmaster SET refresh_token = '', account = '', connected_at = NULL,
		errors = '{}' WHERE id = 1`); err != nil {
		return denmaInternal(err)
	}
	return d.gmailResp(c)
}

// DenmaGmailFetch fetches every domain's figures now.
func (a *App) DenmaGmailFetch(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	d.fetchProviderStats(true)
	return d.gmailResp(c)
}

func (d *denmaCenters) gmailResp(c echo.Context) error {
	v, err := d.gmailView("")
	if err != nil {
		return denmaInternal(err)
	}
	rows, err := d.domainRows("")
	if err != nil {
		return denmaInternal(err)
	}
	return c.JSON(http.StatusOK, okResp{map[string]any{"gmail": v, "domains": rows}})
}

// ViewDenmaGmailConnect sends a superadmin to Google to give the hub read access to Postmaster Tools.
func (a *App) ViewDenmaGmailConnect(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	g, err := d.gmailConfig()
	if err != nil {
		return denmaInternal(err)
	}
	if g.ClientID == "" || g.ClientSecret == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Save the Google client ID and secret first.")
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return denmaInternal(err)
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	if _, err := d.db().Exec(`UPDATE denma.gmail_postmaster SET state = $1, state_at = NOW() WHERE id = 1`, state); err != nil {
		return denmaInternal(err)
	}
	return c.Redirect(http.StatusFound, d.gmailOAuth(g).AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent")))
}

// ViewDenmaGmailCallback takes Google's answer, keeps the lasting access and fetches the figures.
func (a *App) ViewDenmaGmailCallback(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	back := func(notice string) error {
		return c.Redirect(http.StatusFound, path.Join(uriAdmin, "/domains")+"?gmail="+notice)
	}
	g, err := d.gmailConfig()
	if err != nil {
		return denmaInternal(err)
	}
	state := c.QueryParam("state")
	fresh := g.StateAt != nil && time.Since(*g.StateAt) < denmaGmailStateTTL
	if g.State == "" || subtle.ConstantTimeCompare([]byte(state), []byte(g.State)) != 1 || !fresh {
		return back("expired")
	}
	if _, err := d.db().Exec(`UPDATE denma.gmail_postmaster SET state = '', state_at = NULL WHERE id = 1`); err != nil {
		return denmaInternal(err)
	}
	if c.QueryParam("error") != "" {
		return back("denied")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 20*time.Second)
	defer cancel()
	tok, err := d.gmailOAuth(g).Exchange(ctx, c.QueryParam("code"))
	if err != nil || tok.RefreshToken == "" {
		lo.Printf("denma: Google gave no lasting access to Postmaster Tools: %v", denmaGoogleErr(err))
		return back("failed")
	}
	idt, _ := tok.Extra("id_token").(string)
	if _, err := d.db().Exec(`UPDATE denma.gmail_postmaster SET refresh_token = $1, account = $2, connected_at = NOW(),
		fetched_at = NULL, errors = '{}' WHERE id = 1`, tok.RefreshToken, denmaIDTokenEmail(idt)); err != nil {
		return denmaInternal(err)
	}
	go d.fetchProviderStats(true)
	return back("connected")
}

// denmaGoogleErr is an exchange's error, without anything Google sent but its code.
func denmaGoogleErr(err error) string {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		return "oauth2: " + re.ErrorCode
	}
	if err == nil {
		return "no refresh token"
	}
	return "the request failed"
}

// denmaIDTokenEmail is the address in an ID token from Google's token endpoint (” if none).
func denmaIDTokenEmail(idt string) string {
	parts := strings.Split(idt, ".")
	if len(parts) != 3 {
		return ""
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var c struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(b, &c)
	return denmaClip(c.Email, 254)
}

// A center's figures.

// denmaProviderFigures are a center's figures from its own domains (no other center's) for a period.
type denmaProviderFigures struct {
	Domains       int      `db:"domains" json:"domains"`
	SESComplaints *int     `db:"ses_complaints" json:"ses_complaints"`
	SESDeliveries *int     `db:"ses_deliveries" json:"ses_deliveries"`
	GmailAvg      *float64 `db:"gmail_avg" json:"gmail_avg"` // the mean of its days' rates
	GmailWorst    *float64 `db:"gmail_worst" json:"gmail_worst"`
	GmailWorstDay *string  `db:"gmail_worst_day" json:"gmail_worst_day"`
	GmailDays     int      `db:"gmail_days" json:"gmail_days"`
}

// denmaProviderCenterSQL: $1 the center's ID, $2 and $3 the period (by UTC day).
const denmaProviderCenterSQL = `WITH doms AS (
    SELECT dc.domain FROM denma.sending_domain_centers dc WHERE dc.center_id = $1
        AND NOT EXISTS (SELECT 1 FROM denma.sending_domain_centers o WHERE o.domain = dc.domain AND o.center_id <> $1)
),
days AS (
    SELECT p.* FROM denma.provider_days p JOIN doms USING (domain)
    WHERE p.day >= ($2::TIMESTAMPTZ AT TIME ZONE 'UTC')::DATE AND p.day < ($3::TIMESTAMPTZ AT TIME ZONE 'UTC')::DATE
)
SELECT (SELECT COUNT(*) FROM doms) AS domains,
    (SELECT SUM(ses_complaints) FROM days) AS ses_complaints,
    (SELECT SUM(ses_deliveries) FROM days) AS ses_deliveries,
    (SELECT AVG(gmail_spam_rate) FROM days) AS gmail_avg,
    (SELECT MAX(gmail_spam_rate) FROM days) AS gmail_worst,
    (SELECT day::TEXT FROM days WHERE gmail_spam_rate IS NOT NULL ORDER BY gmail_spam_rate DESC, day DESC LIMIT 1) AS gmail_worst_day,
    (SELECT COUNT(*) FROM days WHERE gmail_spam_rate IS NOT NULL) AS gmail_days`

func denmaProvidersFor(db *sqlx.DB, centerID int, from, to time.Time) (*denmaProviderFigures, error) {
	var f denmaProviderFigures
	if err := db.Get(&f, denmaProviderCenterSQL, centerID, from, to); err != nil {
		return nil, err
	}
	return &f, nil
}
