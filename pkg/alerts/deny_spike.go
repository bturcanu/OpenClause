package alerts

import (
	"encoding/json"
	"fmt"
	"time"
)

type DenySpikeConfig struct {
	N        int `json:"n"`
	MMinutes int `json:"m_minutes"`
}

func ParseDenySpikeConfig(raw json.RawMessage) (DenySpikeConfig, error) {
	if len(raw) == 0 {
		return DenySpikeConfig{}, fmt.Errorf("config_json is empty")
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return DenySpikeConfig{}, fmt.Errorf("invalid config_json: %w", err)
	}

	n, ok := getInt(m, "n", "N")
	if !ok {
		return DenySpikeConfig{}, fmt.Errorf("config_json missing n")
	}

	mMinutes, ok := getInt(m, "m_minutes", "m", "window_minutes", "M")
	if !ok {
		return DenySpikeConfig{}, fmt.Errorf("config_json missing m_minutes")
	}

	if n <= 0 {
		return DenySpikeConfig{}, fmt.Errorf("n must be > 0")
	}
	if mMinutes <= 0 {
		return DenySpikeConfig{}, fmt.Errorf("m_minutes must be > 0")
	}
	// Avoid footguns: keep windows within a reasonable operational bound.
	if mMinutes > 24*60 {
		return DenySpikeConfig{}, fmt.Errorf("m_minutes too large")
	}

	return DenySpikeConfig{N: n, MMinutes: mMinutes}, nil
}

func getInt(m map[string]any, keys ...string) (int, bool) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case float64:
			if x != float64(int(x)) {
				return 0, false
			}
			return int(x), true
		case int:
			return x, true
		case json.Number:
			i, err := x.Int64()
			if err != nil {
				return 0, false
			}
			return int(i), true
		default:
			return 0, false
		}
	}
	return 0, false
}

func CountDeniesWithinWindow(denyTimes []time.Time, since time.Time) int {
	count := 0
	for _, t := range denyTimes {
		if !t.Before(since) {
			count++
		}
	}
	return count
}

func FilterDeniesWithinWindow(denyTimes []time.Time, since time.Time) []time.Time {
	out := make([]time.Time, 0, len(denyTimes))
	for _, t := range denyTimes {
		if !t.Before(since) {
			out = append(out, t)
		}
	}
	return out
}

func ShouldFireDenySpike(denyCount int, cfg DenySpikeConfig) bool {
	return denyCount >= cfg.N
}

func ShouldCreateAlertEvent(denyCount int, cfg DenySpikeConfig, existingInWindow bool) bool {
	return ShouldFireDenySpike(denyCount, cfg) && !existingInWindow
}
