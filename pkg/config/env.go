// Package config provides shared environment variable helpers.
package config

import (
	"log/slog"
	"os"
	"strconv"
)

// EnvOr returns the environment variable value or a fallback default.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EnvOrInt returns an integer environment variable or a fallback default.
// Logs a warning if the value is set but not parseable.
func EnvOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		slog.Warn("invalid integer env var, using fallback", "key", key, "value", v, "fallback", fallback)
		return fallback
	}
	return n
}
