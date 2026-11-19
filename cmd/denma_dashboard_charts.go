package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx/types"
)

// denmaDayCountsSQL counts a table's rows per day in time zone $1, for the 31 days up to its latest row's day, every day given.
const denmaDayCountsSQL = `
WITH last AS (SELECT (MAX(created_at) AT TIME ZONE $1)::DATE AS d FROM %[1]s),
days AS (SELECT GENERATE_SERIES(d - 30, d, INTERVAL '1 day')::DATE AS date FROM last WHERE d IS NOT NULL),
n AS (SELECT (created_at AT TIME ZONE $1)::DATE AS date, COUNT(*) AS count FROM %[1]s
	WHERE created_at >= (((SELECT d FROM last) - 30)::TIMESTAMP AT TIME ZONE $1) GROUP BY 1)
SELECT COALESCE(JSON_AGG(JSON_BUILD_OBJECT('date', TO_CHAR(days.date, 'YYYY-MM-DD'), 'count', COALESCE(n.count, 0)) ORDER BY days.date), '[]')
FROM days LEFT JOIN n USING (date)`

// denmaTimeZone is the center's time zone (Config -> General), or UTC.
func (a *App) denmaTimeZone() *time.Location {
	if tz := a.ko.String("denma.timezone"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.UTC
}

// denmaDashboardCharts is the dashboard's views and clicks per day, as the center's time zone counts days.
func (a *App) denmaDashboardCharts() (types.JSONText, error) {
	tz := a.denmaTimeZone().String()
	out := map[string]json.RawMessage{}
	for key, table := range map[string]string{"campaign_views": "campaign_views", "link_clicks": "link_clicks"} {
		var rows json.RawMessage
		if err := a.db.Get(&rows, fmt.Sprintf(denmaDayCountsSQL, table), tz); err != nil {
			return nil, denmaInternal(err)
		}
		out[key] = rows
	}
	b, err := json.Marshal(out)
	return types.JSONText(b), err
}
