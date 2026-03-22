package testdb

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Harness struct {
	pool   *pgxpool.Pool
	schema string
}

func (h *Harness) Pool() *pgxpool.Pool {
	return h.pool
}

func (h *Harness) Schema() string {
	return h.schema
}

func New(t testing.TB) *Harness {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baseDSN := strings.TrimSpace(os.Getenv("OPENCLAUSE_TEST_POSTGRES_DSN"))
	if baseDSN == "" {
		baseDSN = config.PostgresDSN()
	}

	adminCfg, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		t.Fatalf("parse postgres dsn: %v", err)
	}
	adminPool, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Skipf("postgres not available: %v", err)
	}

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		adminPool.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}

	cfg, err := pgxpool.ParseConfig(baseDSN)
	if err != nil {
		adminPool.Close()
		t.Fatalf("parse postgres dsn for test schema: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		adminPool.Close()
		t.Fatalf("open schema-scoped pool: %v", err)
	}
	if err := applyMigrations(ctx, pool); err != nil {
		pool.Close()
		_, _ = adminPool.Exec(context.Background(), `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
		adminPool.Close()
		t.Fatalf("apply migrations: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()

		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()
		_, _ = adminPool.Exec(dropCtx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
		adminPool.Close()
	})

	return &Harness{pool: pool, schema: schema}
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	sqlBytes, err := os.ReadFile(migrationPath())
	if err != nil {
		return err
	}

	for _, stmt := range splitSQLStatements(string(sqlBytes)) {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func migrationPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations", "001_initial.sql")
}

func splitSQLStatements(sqlText string) []string {
	scanner := bufio.NewScanner(strings.NewReader(sqlText))
	statements := make([]string, 0, 256)
	var current strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		current.WriteString(line)
		current.WriteByte('\n')
		if strings.HasSuffix(trimmed, ";") {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
		}
	}

	if tail := strings.TrimSpace(current.String()); tail != "" {
		statements = append(statements, tail)
	}
	return statements
}
