package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
)

const knownInsecureSSLMode = "disable"

var ErrRequiredEnvNotSet = errors.New("required environment variable is not set")

// PostgresDSN builds a Postgres connection string from standard env vars.
// Defaults sslmode to "require"; allows "disable" for local dev but logs a warning.
func PostgresDSN() string {
	sslmode := EnvOr("POSTGRES_SSLMODE", "require")
	if sslmode == knownInsecureSSLMode {
		slog.Warn("POSTGRES_SSLMODE=disable — connection is unencrypted; do not use in production")
	}
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(EnvOr("POSTGRES_USER", "openclause"), EnvOr("POSTGRES_PASSWORD", "changeme")),
		Host:     net.JoinHostPort(EnvOr("POSTGRES_HOST", "localhost"), EnvOr("POSTGRES_PORT", "5432")),
		Path:     EnvOr("POSTGRES_DB", "openclause"),
		RawQuery: "sslmode=" + url.QueryEscape(sslmode) + "&pool_max_conns=" + EnvOr("POSTGRES_POOL_MAX_CONNS", "20"),
	}
	return u.String()
}

// RequireEnv returns the value of an env var or a typed error when it is missing.
func RequireEnv(key string) (string, error) {
	v := EnvOr(key, "")
	if v == "" {
		return "", fmt.Errorf("%w: %s", ErrRequiredEnvNotSet, key)
	}
	return v, nil
}
