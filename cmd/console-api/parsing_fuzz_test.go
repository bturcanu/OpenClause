package main

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func FuzzBearerTokenOnlyReturnsTrimmedSecondField(f *testing.F) {
	for _, seed := range []string{
		"",
		"Bearer token-123",
		" bearer token-123 ",
		"Token token-123",
		"Bearer",
		"Bearer token-123 extra",
		"Bearer\twith-tab",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		fields := strings.Fields(raw)
		want := ""
		if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			want = strings.TrimSpace(fields[1])
		}

		if got := bearerToken(raw); got != want {
			t.Fatalf("bearerToken(%q) = %q, want %q", raw, got, want)
		}
	})
}

func FuzzParseOptionalTimestampNeverReturnsZeroTime(f *testing.F) {
	for _, seed := range []string{
		"",
		"2026-03-23T12:34:56Z",
		"2026-03-23T12:34:56.123456789Z",
		"2026-03-23T12:34",
		"2026-03-23T12:34:56",
		"not-a-date",
		"   2026-03-23T12:34   ",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		got := parseOptionalTimestamp(raw)
		if got != nil && got.IsZero() {
			t.Fatalf("parseOptionalTimestamp(%q) returned zero time", raw)
		}
	})
}

func FuzzParseSinceReturnsFiniteTime(f *testing.F) {
	for _, seed := range []string{
		"",
		"2026-03-23T12:34:56Z",
		"2026-03-23T12:34:56.123456789Z",
		"2026-03-23T12:34",
		"not-a-date",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		req := httptest.NewRequest("GET", "/admin/events", nil)
		if raw != "" {
			values := url.Values{}
			values.Set("since", raw)
			req.URL.RawQuery = values.Encode()
		}

		got := parseSince(req, 24*time.Hour)
		if got.IsZero() {
			t.Fatalf("parseSince(%q) returned zero time", raw)
		}
	})
}

func FuzzParseRangeDurationAlwaysReturnsPositiveDuration(f *testing.F) {
	for _, seed := range []string{
		"",
		"24",
		"48h",
		"7d",
		"0",
		"-5",
		"999999999999999999999",
		"not-a-duration",
		"  12h  ",
		"1.5d",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		req := httptest.NewRequest("GET", "/admin/tenants/demo/analytics/summary", nil)
		if raw != "" {
			values := url.Values{}
			values.Set("range", raw)
			req.URL.RawQuery = values.Encode()
		}

		got := parseRangeDuration(req, 24*time.Hour)
		if got <= 0 {
			t.Fatalf("parseRangeDuration(%q) returned non-positive duration %s", raw, got)
		}
	})
}
