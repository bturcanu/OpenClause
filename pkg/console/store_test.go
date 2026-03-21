package console

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestClampLimit(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 100},
		{-1, 100},
		{50, 50},
		{100, 100},
		{200, 100},
		{1, 1},
	}
	for _, tt := range tests {
		got := clampLimit(tt.input)
		if got != tt.want {
			t.Errorf("clampLimit(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestClampOffset(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{-5, 0},
		{-1, 0},
		{0, 0},
		{10, 10},
		{999, 999},
	}
	for _, tt := range tests {
		got := clampOffset(tt.input)
		if got != tt.want {
			t.Errorf("clampOffset(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func Test_hashInviteResetToken_DeterministicAndDistinct(t *testing.T) {
	s := &Store{tokenHMACSecret: []byte("test-secret")}

	h1 := s.hashInviteResetToken("token-a")
	h2 := s.hashInviteResetToken("token-a")
	h3 := s.hashInviteResetToken("token-b")

	if h1 == "" {
		t.Fatal("expected non-empty hash")
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got h1=%q h2=%q", h1, h2)
	}
	if h1 == h3 {
		t.Fatalf("expected distinct tokens to have distinct hashes: h=%q", h1)
	}
}

func Test_hashInviteResetToken_MatchesExpectedHMACSHA256(t *testing.T) {
	secret := []byte("test-secret")
	raw := "token-a"

	s := &Store{tokenHMACSecret: secret}
	got := s.hashInviteResetToken(raw)

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(raw))
	want := hex.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestCanonicalEmail(t *testing.T) {
	got := canonicalEmail("  User.Name+test@Example.COM ")
	if got != "user.name+test@example.com" {
		t.Fatalf("unexpected canonical email: %q", got)
	}
}

func TestSessionTenantAmbiguityErrorCarriesCandidates(t *testing.T) {
	err := &SessionTenantAmbiguityError{Candidates: []string{"tenant-a", "tenant-b"}}

	if !errors.Is(err, ErrSessionTenantRequired) {
		t.Fatalf("expected ambiguity error to match ErrSessionTenantRequired")
	}
	got := SessionTenantCandidates(err)
	want := []string{"tenant-a", "tenant-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected candidates %v, got %v", want, got)
	}
}

func TestBuildSessionTimelineDeduplicatesBaseRows(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	expires := now.Add(30 * time.Minute)
	success := "success"
	duration := int64(17)
	pending := "pending"
	approvalID := "approval-1"

	rows := []sessionTimelineRow{
		{
			EventID:          "evt-1",
			TenantID:         "tenant-1",
			AgentID:          "agent-1",
			UserID:           "user-1",
			UserName:         "Avery Analyst",
			UserEmail:        "avery@example.com",
			Tool:             "slack",
			Action:           "msg.post",
			Resource:         "#general",
			RiskScore:        8,
			Decision:         "approve",
			SessionID:        "sess-1",
			TraceID:          "trace-1",
			ReceivedAt:       now,
			PayloadJSON:      []byte(`{"risk_factors":["high_impact"]}`),
			PolicyResultJSON: []byte(`{"reason":"risk exceeded threshold"}`),
			ApprovalID:       &approvalID,
			ApprovalStatus:   &pending,
			ApprovalCreated:  &now,
			ApprovalExpires:  &expires,
		},
		{
			EventID:          "evt-1",
			TenantID:         "tenant-1",
			AgentID:          "agent-1",
			UserID:           "user-1",
			UserName:         "Avery Analyst",
			UserEmail:        "avery@example.com",
			Tool:             "slack",
			Action:           "msg.post",
			Resource:         "#general",
			RiskScore:        8,
			Decision:         "approve",
			SessionID:        "sess-1",
			TraceID:          "trace-1",
			ReceivedAt:       now,
			PayloadJSON:      []byte(`{"risk_factors":["high_impact"]}`),
			PolicyResultJSON: []byte(`{"reason":"risk exceeded threshold"}`),
		},
		{
			EventID:          "exec-1",
			ParentEventID:    "evt-1",
			TenantID:         "tenant-1",
			AgentID:          "agent-1",
			Tool:             "slack",
			Action:           "msg.post",
			Decision:         "allow",
			SessionID:        "sess-1",
			ReceivedAt:       now.Add(2 * time.Minute),
			PayloadJSON:      []byte(`{}`),
			PolicyResultJSON: []byte(`{"reason":"approval grant consumed"}`),
			ResultStatus:     &success,
			ResultDuration:   &duration,
		},
	}

	timeline := buildSessionTimeline(rows)
	if len(timeline) != 1 {
		t.Fatalf("expected one timeline item, got %d", len(timeline))
	}
	item := timeline[0]
	if item.EventID != "evt-1" {
		t.Fatalf("expected parent event to remain visible, got %q", item.EventID)
	}
	if item.Approval == nil || item.Approval.ID != "approval-1" {
		t.Fatalf("expected approval summary to be preserved, got %+v", item.Approval)
	}
	if item.Execution == nil || item.Execution.EventID != "exec-1" || item.Execution.Status != "success" {
		t.Fatalf("expected child execution to attach to parent, got %+v", item.Execution)
	}
	if item.Explain == "" {
		t.Fatalf("expected explain text to be generated")
	}
}
