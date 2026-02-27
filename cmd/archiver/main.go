package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bturcanu/OpenClause/pkg/archiver"
	"github.com/bturcanu/OpenClause/pkg/config"
	"github.com/bturcanu/OpenClause/pkg/evidence"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type minioUploader struct {
	client *minio.Client
	bucket string
}

func (m minioUploader) Upload(ctx context.Context, key string, body []byte) error {
	_, err := m.client.PutObject(ctx, m.bucket, key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{
		ContentType: "application/json",
	})
	if err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	return nil
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		config.EnvOr("POSTGRES_USER", "openclause"),
		config.EnvOr("POSTGRES_PASSWORD", "changeme"),
		config.EnvOr("POSTGRES_HOST", "localhost"),
		config.EnvOr("POSTGRES_PORT", "5432"),
		config.EnvOr("POSTGRES_DB", "openclause"),
	)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Error("postgres connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	minioClient, err := minio.New(config.EnvOr("EVIDENCE_S3_ENDPOINT", "localhost:9000"), &minio.Options{
		Creds:  credentials.NewStaticV4(config.EnvOr("EVIDENCE_S3_ACCESS_KEY", "minioadmin"), config.EnvOr("EVIDENCE_S3_SECRET_KEY", "minioadmin"), ""),
		Secure: config.EnvOr("EVIDENCE_S3_SECURE", "false") == "true",
	})
	if err != nil {
		log.Error("minio init failed", "error", err)
		os.Exit(1)
	}

	store := evidence.NewStore(pool)
	svc := archiver.New(store, minioUploader{
		client: minioClient,
		bucket: config.EnvOr("EVIDENCE_S3_BUCKET", "openclause-evidence"),
	})

	onceTenant := os.Getenv("ARCHIVER_TENANT_ID")
	runOnce := config.EnvOr("ARCHIVER_RUN_ONCE", "true") == "true"
	interval := time.Duration(config.EnvOrInt("ARCHIVER_INTERVAL_SEC", 300)) * time.Second

	run := func() {
		tenants := []string{}
		if onceTenant != "" {
			tenants = append(tenants, onceTenant)
		} else {
			all, err := store.ListTenantIDs(ctx)
			if err != nil {
				log.Error("list tenants failed", "error", err)
				return
			}
			tenants = all
		}
		for _, tenantID := range tenants {
			key, err := svc.ArchiveTenant(ctx, tenantID)
			if err != nil {
				log.Error("archive tenant failed", "tenant_id", tenantID, "error", err)
				continue
			}
			if key != "" {
				log.Info("archived evidence bundle", "tenant_id", tenantID, "key", key)
			}
		}
	}

	run()
	if runOnce {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
