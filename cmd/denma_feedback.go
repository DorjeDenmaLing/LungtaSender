package main

// denma: e-mails sent to providers that report complaints to Amazon SES, for complaint rates counted as SES counts them.

import (
	"net/http"
	"strings"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// denmaFeedbackProviders are the providers (denmaProviders) that report complaints; Gmail and Apple don't.
var denmaFeedbackProviders = []string{"microsoft", "yahoo"}

// denmaFeedbackDomains are other domains that report complaints.
var denmaFeedbackDomains = []string{"comcast.net"}

// denmaFeedbackSQL is whether the address in col is at a provider that reports complaints.
func denmaFeedbackSQL(col string) string {
	return "(" + denmaProviderSQL(col) + " IN (" + denmaQuoteAll(denmaFeedbackProviders) + ") OR LOWER(SPLIT_PART(" + col +
		", '@', -1)) IN (" + denmaQuoteAll(denmaFeedbackDomains) + "))"
}

func denmaQuoteAll(vals []string) string {
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = pq.QuoteLiteral(v)
	}
	return strings.Join(q, ", ")
}

// denmaFeedbackSchemaSQL adds the flag to the sent e-mails record, and the days kept once it's pruned (in denmaFeaturesSQL).
const denmaFeedbackSchemaSQL = `
ALTER TABLE denma_messages ADD COLUMN IF NOT EXISTS feedback BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS denma_feedback_days (
    day   DATE PRIMARY KEY, -- UTC
    sends INTEGER NOT NULL DEFAULT 0
);
`

// denmaFeedbackBackfillSQL flags the e-mails recorded before the flag.
func denmaFeedbackBackfillSQL() string {
	return `UPDATE denma_messages m SET feedback = true FROM subscribers s
		WHERE s.id = m.subscriber_id AND NOT m.feedback AND ` + denmaFeedbackSQL("s.email")
}

// denmaFeedbackSent is the condition for a recorded e-mail (alias m) that counts.
const denmaFeedbackSent = `m.feedback AND m.status IN ('sent', 'uncertain') AND m.kind <> 'test'`

// denmaFeedbackPruneSQL keeps the day counts of the e-mails it deletes; $1 is denmaMessageDays.
const denmaFeedbackPruneSQL = `WITH days AS (
	INSERT INTO denma_feedback_days (day, sends)
	SELECT (m.sent_at AT TIME ZONE 'UTC')::DATE, COUNT(*) FROM denma_messages m
	WHERE m.sent_at < NOW() - make_interval(days => $1) AND ` + denmaFeedbackSent + `
	GROUP BY 1
	ON CONFLICT (day) DO UPDATE SET sends = denma_feedback_days.sends + EXCLUDED.sends
)
DELETE FROM denma_messages WHERE sent_at < NOW() - make_interval(days => $1)`

// denmaFeedbackSendsSQL counts the e-mails sent from $1 to $2 to providers that report complaints; S. is the schema.
const denmaFeedbackSendsSQL = `(SELECT COUNT(*) FROM S.denma_messages m WHERE ` + denmaFeedbackSent + ` AND m.sent_at >= $1 AND m.sent_at < $2)
	+ (SELECT COALESCE(SUM(sends), 0) FROM S.denma_feedback_days WHERE day >= ($1 AT TIME ZONE 'UTC')::DATE AND day < ($2 AT TIME ZONE 'UTC')::DATE)`

// denmaFeedbackSinceSQL is when the record of e-mails starts (NULL without any); complaints before it aren't counted.
const denmaFeedbackSinceSQL = `LEAST((SELECT MIN(sent_at) FROM S.denma_messages), (SELECT MIN(day)::TIMESTAMP AT TIME ZONE 'UTC' FROM S.denma_feedback_days))`

// denmaFeedbackComplaintsSQL counts the complaints from $1 to $2 since the record starts.
const denmaFeedbackComplaintsSQL = `(SELECT COUNT(*) FROM S.bounces WHERE type = 'complaint' AND created_at >= GREATEST($1, ` + denmaFeedbackSinceSQL + `) AND created_at < $2)`

// DenmaStatFeedback returns the e-mails sent to providers that report complaints, in a period and the one before.
func (a *App) DenmaStatFeedback(c echo.Context) error {
	if u := auth.GetUser(c); !denmaAllLists(&u) {
		return echo.NewHTTPError(http.StatusForbidden, "This needs access to every list.")
	}
	t, err := denmaPeriods(c)
	if err != nil {
		return err
	}
	var out struct {
		Cur   int        `json:"cur"`
		Prev  int        `json:"prev"`
		Since *time.Time `json:"since"` // complaints before it have no e-mails to count against
		// SES's and Gmail's own figures for the center's domains (cmd/denma_provider_stats.go).
		Providers map[string]*denmaProviderFigures `json:"providers"`
	}
	q := `SELECT ` + strings.ReplaceAll(denmaFeedbackSendsSQL, "S.", "")
	for i, n := range []*int{&out.Cur, &out.Prev} {
		if err := a.db.Get(n, q, t[2*i], t[2*i+1]); err != nil {
			return denmaInternal(err)
		}
	}
	if err := a.db.Get(&out.Since, `SELECT `+strings.ReplaceAll(denmaFeedbackSinceSQL, "S.", "")); err != nil {
		return denmaInternal(err)
	}
	if ctr := a.denmaCenterOf(); ctr != nil {
		out.Providers = map[string]*denmaProviderFigures{}
		for i, k := range []string{"cur", "prev"} {
			f, err := denmaProvidersFor(a.db, ctr.ID, t[2*i], t[2*i+1])
			if err != nil {
				return denmaInternal(err)
			}
			out.Providers[k] = f
		}
	}
	return c.JSON(http.StatusOK, okResp{out})
}
