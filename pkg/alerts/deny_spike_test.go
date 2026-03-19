package alerts

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDenySpikeConfig_NAndMMinutesLowercase(t *testing.T) {
	raw := json.RawMessage(`{"n":3,"m_minutes":5}`)
	cfg, err := ParseDenySpikeConfig(raw)
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if cfg.N != 3 || cfg.MMinutes != 5 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestParseDenySpikeConfig_NAndMMixedKeys(t *testing.T) {
	raw := json.RawMessage(`{"N":4,"M":7}`)
	cfg, err := ParseDenySpikeConfig(raw)
	if err != nil {
		t.Fatalf("expected success, got err=%v", err)
	}
	if cfg.N != 4 || cfg.MMinutes != 7 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestParseDenySpikeConfig_RejectsNonPositive(t *testing.T) {
	raw := json.RawMessage(`{"n":0,"m_minutes":5}`)
	_, err := ParseDenySpikeConfig(raw)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterDeniesWithinWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)
	denyTimes := []time.Time{
		since.Add(-1 * time.Second),
		since.Add(1 * time.Second),
		now.Add(-1 * time.Minute),
		now.Add(1 * time.Second), // should still count as within window
	}
	out := FilterDeniesWithinWindow(denyTimes, since)
	if len(out) != 3 {
		t.Fatalf("expected 3 denies within window, got %d", len(out))
	}
}

func TestCountDeniesWithinWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-5 * time.Minute)
	denyTimes := []time.Time{
		since.Add(-1 * time.Second),
		since.Add(1 * time.Second),
		now.Add(-1 * time.Minute),
	}
	if got := CountDeniesWithinWindow(denyTimes, since); got != 2 {
		t.Fatalf("expected 2 denies within window, got %d", got)
	}
}

func TestShouldCreateAlertEvent_DedupeBlocks(t *testing.T) {
	cfg := DenySpikeConfig{N: 3, MMinutes: 5}
	denyCount := 3
	if ShouldCreateAlertEvent(denyCount, cfg, true) {
		t.Fatal("expected dedupe to block creation")
	}
	if !ShouldCreateAlertEvent(denyCount, cfg, false) {
		t.Fatal("expected creation when no existing event exists")
	}
}
