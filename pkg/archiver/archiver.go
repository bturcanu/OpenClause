package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bturcanu/OpenClause/pkg/evidence"
)

type EvidenceStore interface {
	GetArchiveCheckpoint(context.Context, string) (time.Time, string, int64, error)
	GetChainEvents(context.Context, string, int64) ([]evidence.ChainEvent, error)
	UpsertArchiveCheckpoint(context.Context, string, time.Time, string, int64) error
	ListTenantIDs(context.Context) ([]string, error)
}

type Uploader interface {
	Upload(ctx context.Context, key string, body []byte) error
}

type Service struct {
	store    EvidenceStore
	uploader Uploader
}

func New(store EvidenceStore, uploader Uploader) *Service {
	return &Service{store: store, uploader: uploader}
}

type Bundle struct {
	TenantID     string                `json:"tenant_id"`
	CreatedAt    time.Time             `json:"created_at"`
	EventCount   int                   `json:"event_count"`
	Checkpoint   string                `json:"checkpoint_hash"`
	Since        time.Time             `json:"since"`
	Until        time.Time             `json:"until"`
	ChainRecords []evidence.ChainEvent `json:"chain_records"`
}

func (s *Service) ArchiveTenant(ctx context.Context, tenantID string) (string, error) {
	since, lastHash, lastSeq, err := s.store.GetArchiveCheckpoint(ctx, tenantID)
	if err != nil {
		return "", err
	}
	events, err := s.store.GetChainEvents(ctx, tenantID, lastSeq)
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return "", nil
	}
	if err := evidence.VerifyChainFrom(lastHash, events); err != nil {
		return "", fmt.Errorf("verify chain: %w", err)
	}

	last := events[len(events)-1]
	now := time.Now().UTC()
	checkpointAt := events[len(events)-1].ReceivedAt
	bundle := Bundle{
		TenantID:     tenantID,
		CreatedAt:    now,
		EventCount:   len(events),
		Checkpoint:   last.Hash,
		Since:        since,
		Until:        checkpointAt,
		ChainRecords: events,
	}
	body, err := json.Marshal(bundle)
	if err != nil {
		return "", fmt.Errorf("marshal bundle: %w", err)
	}

	key := fmt.Sprintf("evidence/%s/%04d/%02d/%02d/%s.json", tenantID, now.Year(), now.Month(), now.Day(), last.Hash)
	if err := s.uploader.Upload(ctx, key, body); err != nil {
		return "", err
	}
	if err := s.store.UpsertArchiveCheckpoint(ctx, tenantID, checkpointAt, last.Hash, last.EventSeq); err != nil {
		return "", err
	}
	return key, nil
}
