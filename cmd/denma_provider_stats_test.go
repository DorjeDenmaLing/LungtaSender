package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Gmail's daily spam rates are read across pages, by metric and date.
func TestDenmaGmailDays(t *testing.T) {
	var asked []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/domains/example.org/domainStats:query" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(b, &req)
		asked = append(asked, req)
		if req["pageToken"] == nil {
			_, _ = io.WriteString(w, `{"domainStats":[
				{"metric":"spam","date":{"year":2026,"month":11,"day":17},"value":{"doubleValue":0.0046}},
				{"metric":"other","date":{"year":2026,"month":11,"day":17},"value":{"doubleValue":9}},
				{"metric":"spam","value":{"doubleValue":1}}
			],"nextPageToken":"p2"}`)
			return
		}
		_, _ = io.WriteString(w, `{"domainStats":[{"metric":"spam","date":{"year":2026,"month":11,"day":18},"value":{"floatValue":0.001}}]}`)
	}))
	defer srv.Close()
	defer func(u string) { denmaGmailAPI = u }(denmaGmailAPI)
	denmaGmailAPI = srv.URL

	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	days, err := denmaGmailDays(context.Background(), srv.Client(), "example.org", from, to, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []denmaGmailDay{{"2026-11-17", 0.0046}, {"2026-11-18", 0.001}}
	if len(days) != len(want) || days[0] != want[0] || days[1] != want[1] {
		t.Fatalf("days = %v, want %v", days, want)
	}
	if len(asked) != 2 || asked[1]["pageToken"] != "p2" || asked[0]["aggregationGranularity"] != "DAILY" {
		t.Fatalf("requests = %v", asked)
	}
	tq, _ := json.Marshal(asked[0]["timeQuery"])
	if !strings.Contains(string(tq), `"end":{"day":19,"month":11,"year":2026}`) {
		t.Fatalf("timeQuery = %s", tq)
	}
}

// A domain Postmaster Tools doesn't have for the account says so; other refusals give Google's message.
func TestDenmaGmailDaysRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "missing.org") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"Quota exceeded."}}`)
	}))
	defer srv.Close()
	defer func(u string) { denmaGmailAPI = u }(denmaGmailAPI)
	denmaGmailAPI = srv.URL

	now := time.Now().UTC()
	_, err := denmaGmailDays(context.Background(), srv.Client(), "missing.org", now, now, "ops@example.org")
	if err == nil || !strings.Contains(err.Error(), "Not in Gmail Postmaster Tools for ops@example.org") {
		t.Fatalf("err = %v", err)
	}
	_, err = denmaGmailDays(context.Background(), srv.Client(), "busy.org", now, now, "")
	if err == nil || !strings.Contains(err.Error(), "429: Quota exceeded.") {
		t.Fatalf("err = %v", err)
	}
}

func TestDenmaIDTokenEmail(t *testing.T) {
	part := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tok := part(`{"alg":"RS256"}`) + "." + part(`{"email":"ops@example.org"}`) + ".sig"
	if got := denmaIDTokenEmail(tok); got != "ops@example.org" {
		t.Fatalf("email = %q", got)
	}
	for _, bad := range []string{"", "a.b", "a.!!.c", part("{}") + "." + part("[]") + ".s"} {
		if got := denmaIDTokenEmail(bad); got != "" {
			t.Fatalf("denmaIDTokenEmail(%q) = %q", bad, got)
		}
	}
}

// A fetch starts a few days before the domain's last, within the backfill; a new domain's at the backfill.
func TestDenmaProviderFrom(t *testing.T) {
	today := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	through := map[string]time.Time{"a.org": today, "b.org": today.AddDate(0, 0, -20), "c.org": today.AddDate(0, 0, -200)}
	for dom, want := range map[string]time.Time{
		"a.org": today.AddDate(0, 0, -denmaProviderRecent),
		"b.org": today.AddDate(0, 0, -20-denmaProviderRecent),
		"c.org": today.AddDate(0, 0, -denmaProviderBackfill),
		"d.org": today.AddDate(0, 0, -denmaProviderBackfill),
	} {
		if got := denmaProviderFrom(through, dom, today); !got.Equal(want) {
			t.Fatalf("%s from %v, want %v", dom, got, want)
		}
	}
}

// A fetch is split into spans SES takes, the last one shorter.
func TestDenmaDayWindows(t *testing.T) {
	day := func(m, d int) time.Time { return time.Date(2026, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	got := denmaDayWindows(day(8, 23), day(11, 22), denmaVDMMaxDays)
	if len(got) != 2 || !got[0][0].Equal(day(8, 23)) || !got[0][1].Equal(day(10, 22)) || !got[1][0].Equal(day(10, 22)) || !got[1][1].Equal(day(11, 22)) {
		t.Fatalf("90 days = %v", got)
	}
	if got := denmaDayWindows(day(9, 1), day(10, 31), 60); len(got) != 1 {
		t.Fatalf("60 days = %v", got)
	}
	if got := denmaDayWindows(day(9, 1), day(9, 1), 60); len(got) != 0 {
		t.Fatalf("no days = %v", got)
	}
}
