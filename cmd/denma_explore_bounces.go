package main

import (
	"time"

	"github.com/lib/pq"
)

// denmaExploreBounce is one bucket's hard or soft bounces of a campaign or automation.
type denmaExploreBounce struct {
	CampaignID   int       `db:"campaign_id" json:"campaign_id,omitempty"`
	AutomationID int       `db:"automation_id" json:"automation_id,omitempty"`
	Type         string    `db:"type" json:"type"`
	Count        int       `db:"count" json:"count"`
	Timestamp    time.Time `db:"timestamp" json:"timestamp"`
}

// denmaExploreComplaint is a complaint on Explore; who made it only for those who can see bounces.
type denmaExploreComplaint struct {
	At           time.Time `db:"created_at"`
	CampaignID   int       `db:"campaign_id"`
	AutomationID int       `db:"automation_id"`
	Of           string    `db:"of"` // the campaign's or automation's name
	SubscriberID int       `db:"subscriber_id"`
	Name         string    `db:"name"`
	Email        string    `db:"email"`
}

// Day is the complaint's day in the center's time zone.
func (c denmaExploreComplaint) Day(loc *time.Location) string {
	return c.At.In(loc).Format("Jan 2, 2006")
}

// Time is the complaint's time of day in the center's time zone.
func (c denmaExploreComplaint) Time(loc *time.Location) string {
	return c.At.In(loc).Format("3:04 PM MST")
}

const denmaExploreComplaintsMax = 500

// denmaAnalyticsStamp writes Explore's range for a query, with its offset.
const denmaAnalyticsStamp = "2006-01-02 15:04:05-07:00"

// denmaExploreBucket is created_at's day (a range of 7 days or more) or hour, in the time zone of parameter tz.
func denmaExploreBucket(tz string) string {
	return `DATE_TRUNC(CASE WHEN (EXTRACT (EPOCH FROM ($3::TIMESTAMPTZ - $2::TIMESTAMPTZ)) / 86400) >= 7 THEN 'day' ELSE 'hour' END, created_at, ` + tz + `)`
}

// denmaExploreBounces gives Explore its campaigns' and automations' bounces by type, and their complaints apart.
func (a *App) denmaExploreBounces(v *campaignAnalyticsView, from, to time.Time) error {
	d := &v.Denma
	d.TypedBounces, d.Complaints = []denmaExploreBounce{}, []denmaExploreComplaint{}
	d.ComplaintsWho = v.Can("bounces:get") && v.DenmaAllLists()
	d.ComplaintsMax = denmaExploreComplaintsMax
	d.Zone = a.denmaTimeZone()
	d.TimeZone = d.Zone.String()
	camps := make([]int, len(v.Campaigns))
	for i, cm := range v.Campaigns {
		camps[i] = cm.ID
	}
	autos := make([]int, len(d.Automations))
	for i, au := range d.Automations {
		autos[i] = au.ID
	}
	if len(camps) == 0 && len(autos) == 0 {
		return nil
	}
	var (
		fromStr = from.Format(denmaAnalyticsStamp)
		toStr   = to.Format(denmaAnalyticsStamp)
	)
	if err := a.db.Select(&d.TypedBounces, `
		SELECT campaign_id, 0 AS automation_id, type::TEXT AS type, COUNT(*) AS count, `+denmaExploreBucket("$5")+` AS timestamp
		FROM bounces WHERE campaign_id = ANY($1) AND type <> 'complaint' AND created_at >= $2 AND created_at <= $3 GROUP BY 1, 3, 5
		UNION ALL
		SELECT 0, automation_id, type, COUNT(*), `+denmaExploreBucket("$5")+`
		FROM denma_automation_bounces WHERE automation_id = ANY($4) AND type <> 'complaint' AND created_at >= $2 AND created_at <= $3 GROUP BY 2, 3, 5
		ORDER BY timestamp`, pq.Array(camps), fromStr, toStr, pq.Array(autos), d.TimeZone); err != nil {
		return denmaInternal(err)
	}
	if err := a.db.Select(&d.Complaints, `
		SELECT * FROM (
			SELECT b.created_at, b.campaign_id, 0 AS automation_id, c.name AS of, b.subscriber_id, s.name, s.email
			FROM bounces b JOIN campaigns c ON c.id = b.campaign_id JOIN subscribers s ON s.id = b.subscriber_id
			WHERE b.campaign_id = ANY($1) AND b.type = 'complaint' AND b.created_at >= $2 AND b.created_at <= $3
			UNION ALL
			SELECT ab.created_at, 0, ab.automation_id, au.name, 0, '', ''
			FROM denma_automation_bounces ab JOIN denma_automations au ON au.id = ab.automation_id
			WHERE ab.automation_id = ANY($4) AND ab.type = 'complaint' AND ab.created_at >= $2 AND ab.created_at <= $3
		) x ORDER BY created_at DESC LIMIT $5`, pq.Array(camps), fromStr, toStr, pq.Array(autos), denmaExploreComplaintsMax+1); err != nil {
		return denmaInternal(err)
	}
	var n int
	if err := a.db.Get(&n, `SELECT
		(SELECT COUNT(*) FROM bounces WHERE campaign_id = ANY($1) AND type = 'complaint' AND created_at >= $2 AND created_at <= $3) +
		(SELECT COUNT(*) FROM denma_automation_bounces WHERE automation_id = ANY($4) AND type = 'complaint' AND created_at >= $2 AND created_at <= $3)`,
		pq.Array(camps), fromStr, toStr, pq.Array(autos)); err != nil {
		return denmaInternal(err)
	}
	d.ComplaintCount = n
	if len(d.Complaints) > denmaExploreComplaintsMax {
		d.Complaints = d.Complaints[:denmaExploreComplaintsMax]
	}
	if !d.ComplaintsWho {
		for i := range d.Complaints {
			d.Complaints[i].SubscriberID, d.Complaints[i].Name, d.Complaints[i].Email = 0, "", ""
		}
	}

	v.Counts.Bounces = 0
	for _, b := range d.TypedBounces {
		v.Counts.Bounces += b.Count
	}
	if len(d.TypedBounces) > 0 || n > 0 {
		v.HasData = true
	}
	return nil
}
