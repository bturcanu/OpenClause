package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
)

const knownInsecureSSLMode = "disable"

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

// RequireEnv returns the value of an env var or calls fatal via the provided callback.
func RequireEnv(key string) string {
	v := EnvOr(key, "")
	if v == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return v
}
