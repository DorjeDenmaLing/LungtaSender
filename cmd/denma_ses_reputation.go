package main

// denma: the SES account's status and its bounce and complaint rates (AWS/SES Reputation.*), on the hub's dashboard.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/labstack/echo/v4"
)

// denmaSESReputation is the SES account's health as SES judges it.
type denmaSESReputation struct {
	Status         string     `json:"status"` // HEALTHY, PROBATION or SHUTDOWN
	SendingEnabled bool       `json:"sending_enabled"`
	BounceRate     *float64   `json:"bounce_rate"` // a share (0.02 is 2%); nil until SES has one
	ComplaintRate  *float64   `json:"complaint_rate"`
	At             *time.Time `json:"at"` // the rates' latest figure
}

// denmaSESReputationTTL is how long the figures are kept; SES updates them a few times a day.
const denmaSESReputationTTL = 5 * time.Minute

// denmaSESReputationDays is how far back the latest rate is looked for.
const denmaSESReputationDays = 14

func (s *denmaRealSES) Reputation(ctx context.Context) (denmaSESReputation, error) {
	var r denmaSESReputation
	acc, err := s.v2.GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		return r, err
	}
	r.Status, r.SendingEnabled = aws.ToString(acc.EnforcementStatus), acc.SendingEnabled

	q := func(id, name string) cwtypes.MetricDataQuery {
		return cwtypes.MetricDataQuery{Id: aws.String(id), MetricStat: &cwtypes.MetricStat{
			Metric: &cwtypes.Metric{Namespace: aws.String("AWS/SES"), MetricName: aws.String(name)},
			Period: aws.Int32(3600), Stat: aws.String("Average"),
		}}
	}
	end := time.Now()
	out, err := s.cw.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		StartTime:         aws.Time(end.AddDate(0, 0, -denmaSESReputationDays)),
		EndTime:           aws.Time(end),
		ScanBy:            cwtypes.ScanByTimestampDescending,
		MetricDataQueries: []cwtypes.MetricDataQuery{q("bounce", "Reputation.BounceRate"), q("complaint", "Reputation.ComplaintRate")},
	})
	if err != nil {
		return r, err
	}
	for _, m := range out.MetricDataResults {
		if len(m.Values) == 0 || len(m.Timestamps) == 0 {
			continue
		}
		v, at := m.Values[0], m.Timestamps[0]
		switch aws.ToString(m.Id) {
		case "bounce":
			r.BounceRate = &v
		case "complaint":
			r.ComplaintRate = &v
		}
		if r.At == nil || at.After(*r.At) {
			r.At = &at
		}
	}
	return r, nil
}

// Reputation is the dev account's (denma.dev_ses_account), healthy with no rates until one is set.
func (s *denmaFakeSES) Reputation(ctx context.Context) (denmaSESReputation, error) {
	r := denmaSESReputation{Status: "HEALTHY", SendingEnabled: true}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS denma.dev_ses_account (id INT PRIMARY KEY CHECK (id = 1), data JSONB NOT NULL)`); err != nil {
		return r, err
	}
	var b []byte
	err := s.db.GetContext(ctx, &b, `SELECT data FROM denma.dev_ses_account WHERE id = 1`)
	if err == sql.ErrNoRows {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

// DenmaHubSES returns the SES account's figures, or why there are none.
func (a *App) DenmaHubSES(c echo.Context) error {
	d, err := a.hub(c)
	if err != nil {
		return err
	}
	type resp struct {
		denmaSESReputation
		Available bool   `json:"available"`
		Reason    string `json:"reason,omitempty"`
	}
	s, why := d.ses()
	if s == nil {
		return c.JSON(http.StatusOK, okResp{resp{Reason: why}})
	}
	read := func() (denmaSESReputation, error) {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
		defer cancel()
		return s.Reputation(ctx)
	}
	var r denmaSESReputation
	if _, dev := s.(*denmaFakeSES); dev { // the dev SES's figures, as a check sets them
		r, err = read()
	} else {
		r, err = denmaCachedFor("ses|reputation", denmaSESReputationTTL, read)
	}
	if err != nil {
		a.log.Printf("denma: error reading the SES account's figures: %v", err)
		return c.JSON(http.StatusOK, okResp{resp{Reason: "Amazon SES's figures couldn't be read."}})
	}
	return c.JSON(http.StatusOK, okResp{resp{denmaSESReputation: r, Available: true}})
}
