package main

import "testing"

// A client's address counts as itself (IPv6 by its /64); anything else isn't one.
func TestDenmaMCPClientBucket(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7":           "client:203.0.113.7",
		" 203.0.113.7 ":         "client:203.0.113.7",
		"::ffff:203.0.113.7":    "client:203.0.113.7",
		"2001:db8:1:2:3:4:5:6":  "client:2001:db8:1:2::/64",
		"":                      "",
		"unknown":               "",
		"203.0.113.7, 10.0.0.1": "",
		"secret:abc":            "",
		"203.0.113.7:8080":      "",
	} {
		if got := denmaMCPClientBucket(in); got != want {
			t.Errorf("denmaMCPClientBucket(%q) = %q, want %q", in, got, want)
		}
	}
}
