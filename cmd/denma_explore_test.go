package main

import (
	"testing"
	"time"
)

// Explore's From and To are read in the center's time zone, and sent with its offset on that day.
func TestDenmaExploreRangeZone(t *testing.T) {
	loc, err := time.LoadLocation("America/Halifax")
	if err != nil {
		t.Skip("no time zone data")
	}
	for in, want := range map[string]string{
		"2026-07-01T22:00": "2026-07-01 22:00:00-03:00",
		"2026-11-27T22:00": "2026-11-27 22:00:00-04:00",
		"2026-11-01T23:30": "2026-11-01 23:30:00-04:00",
	} {
		got, ok := parseAnalyticsDate(in, loc)
		if !ok || got.Format(denmaAnalyticsStamp) != want {
			t.Errorf("%s: got %s (%v), want %s", in, got.Format(denmaAnalyticsStamp), ok, want)
		}
	}
	if _, ok := parseAnalyticsDate("2026-11-27", loc); ok {
		t.Error("a date without a time was read")
	}
}
