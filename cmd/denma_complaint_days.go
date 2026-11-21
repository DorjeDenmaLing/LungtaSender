package main

// denma: a center's complaints by UTC day, with SES's and Gmail's own figures for its domains (Analytics -> Complaints).

import (
	"net/http"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/labstack/echo/v4"
)

// denmaComplaintDaysMax is the longest period given, in days.
const denmaComplaintDaysMax = 400

// denmaComplaintDay is one UTC day's figures.
type denmaComplaintDay struct {
	Day           string   `db:"day" json:"day"`
	Sent          int      `db:"sent" json:"sent"`
	Complaints    int      `db:"complaints" json:"complaints"`
	SESComplaints *int     `db:"ses_complaints" json:"ses_complaints"`
	SESDeliveries *int     `db:"ses_deliveries" json:"ses_deliveries"`
	GmailRate     *float64 `db:"gmail_spam_rate" json:"gmail_spam_rate"`
}

// denmaComplaintDaysSQL: $1 the center's ID, $2 and $3 the first day and the day after the last.
const denmaComplaintDaysSQL = `WITH doms AS (
    SELECT dc.domain FROM denma.sending_domain_centers dc WHERE dc.center_id = $1
        AND NOT EXISTS (SELECT 1 FROM denma.sending_domain_centers o WHERE o.domain = dc.domain AND o.center_id <> $1)
),
days AS (SELECT d::DATE AS day FROM generate_series($2::DATE, $3::DATE - 1, INTERVAL '1 day') d),
sent AS (
    SELECT (sent_at AT TIME ZONE 'UTC')::DATE AS day, COUNT(*) AS n FROM denma_messages
    WHERE status IN ('sent', 'uncertain') AND kind <> 'test'
        AND sent_at >= $2::DATE::TIMESTAMP AT TIME ZONE 'UTC' AND sent_at < $3::DATE::TIMESTAMP AT TIME ZONE 'UTC'
    GROUP BY 1
),
comp AS (
    SELECT (created_at AT TIME ZONE 'UTC')::DATE AS day, COUNT(*) AS n FROM bounces
    WHERE type = 'complaint' AND created_at >= $2::DATE::TIMESTAMP AT TIME ZONE 'UTC' AND created_at < $3::DATE::TIMESTAMP AT TIME ZONE 'UTC'
    GROUP BY 1
),
prov AS (
    SELECT p.day, SUM(p.ses_complaints) AS ses_complaints, SUM(p.ses_deliveries) AS ses_deliveries, AVG(p.gmail_spam_rate) AS gmail_spam_rate
    FROM denma.provider_days p JOIN doms USING (domain) WHERE p.day >= $2::DATE AND p.day < $3::DATE GROUP BY 1
)
SELECT days.day::TEXT AS day, COALESCE(sent.n, 0) AS sent, COALESCE(comp.n, 0) AS complaints,
    prov.ses_complaints, prov.ses_deliveries, prov.gmail_spam_rate
FROM days LEFT JOIN sent USING (day) LEFT JOIN comp USING (day) LEFT JOIN prov USING (day) ORDER BY days.day`

// DenmaStatComplaintDays returns a period's complaints and providers' figures by UTC day.
func (a *App) DenmaStatComplaintDays(c echo.Context) error {
	if u := auth.GetUser(c); !denmaAllLists(&u) {
		return echo.NewHTTPError(http.StatusForbidden, "This needs access to every list.")
	}
	ctr := a.denmaCenterOf()
	if ctr == nil {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	var t [2]time.Time
	for i, k := range []string{"from", "to"} {
		v, err := time.Parse(time.RFC3339, c.QueryParam(k))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid "+k)
		}
		t[i] = v.UTC().Truncate(24 * time.Hour)
	}
	if t[1].Before(t[0]) || t[1].Sub(t[0]) > denmaComplaintDaysMax*24*time.Hour {
		return echo.NewHTTPError(http.StatusBadRequest, "The period has to be a year or less.")
	}
	if t[1].Equal(t[0]) {
		t[1] = t[0].AddDate(0, 0, 1)
	}
	var out struct {
		Domains   int                 `json:"domains"`    // the center's own sending domains
		SentSince *time.Time          `json:"sent_since"` // the record of e-mails sent starts
		Days      []denmaComplaintDay `json:"days"`
	}
	if err := a.db.Get(&out.Domains, `SELECT COUNT(*) FROM denma.sending_domain_centers dc WHERE dc.center_id = $1
		AND NOT EXISTS (SELECT 1 FROM denma.sending_domain_centers o WHERE o.domain = dc.domain AND o.center_id <> $1)`, ctr.ID); err != nil {
		return denmaInternal(err)
	}
	if err := a.db.Get(&out.SentSince, `SELECT MIN(sent_at) FROM denma_messages`); err != nil {
		return denmaInternal(err)
	}
	day := func(x time.Time) string { return x.Format("2006-01-02") }
	if err := a.db.Select(&out.Days, denmaComplaintDaysSQL, ctr.ID, day(t[0]), day(t[1])); err != nil {
		return denmaInternal(err)
	}
	return c.JSON(http.StatusOK, okResp{out})
}
