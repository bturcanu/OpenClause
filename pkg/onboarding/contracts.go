package onboarding

type IntegrationInput struct {
	Runtime          string         `json:"runtime"`
	TenantID         string         `json:"tenant_id,omitempty"`
	NewTenantName    string         `json:"new_tenant_name,omitempty"`
	AgentName        string         `json:"agent_name"`
	EnvironmentLabel string         `json:"environment_label,omitempty"`
	OwnerName        string         `json:"owner_name,omitempty"`
	Description      string         `json:"description,omitempty"`
	ApprovalPosture  string         `json:"approval_posture,omitempty"`
	Tools            []SelectedTool `json:"tools"`
}

type RegenerateInput struct {
	IntegrationInput
	AgentID string `json:"agent_id"`
}

type BundleDefault struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Reason string `json:"reason,omitempty"`
}

type BundleAgent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	Preview   bool   `json:"preview"`
}

type BundleTenant struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

type BundleAPIKey struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	KeyPrefix string  `json:"key_prefix"`
	RawKey    string  `json:"raw_key,omitempty"`
	ExpiresAt *string `json:"expires_at,omitempty"`
}

type BundleIntegration struct {
	ID               string         `json:"id"`
	TenantID         string         `json:"tenant_id"`
	AgentID          string         `json:"agent_id"`
	Runtime          string         `json:"runtime"`
	EnvironmentLabel string         `json:"environment_label,omitempty"`
	OwnerName        string         `json:"owner_name,omitempty"`
	Description      string         `json:"description,omitempty"`
	ApprovalPosture  string         `json:"approval_posture,omitempty"`
	Tools            []SelectedTool `json:"tools,omitempty"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
}

type BundleIntegrationRevision struct {
	ID               string         `json:"id"`
	IntegrationID    string         `json:"integration_id"`
	TenantID         string         `json:"tenant_id"`
	AgentID          string         `json:"agent_id"`
	Mode             string         `json:"mode"`
	Runtime          string         `json:"runtime"`
	EnvironmentLabel string         `json:"environment_label,omitempty"`
	OwnerName        string         `json:"owner_name,omitempty"`
	Description      string         `json:"description,omitempty"`
	ApprovalPosture  string         `json:"approval_posture,omitempty"`
	Tools            []SelectedTool `json:"tools,omitempty"`
	CreatedAt        string         `json:"created_at"`
}

type BundleResponse struct {
	Mode        string             `json:"mode"`
	Tenant      BundleTenant       `json:"tenant"`
	Agent       BundleAgent        `json:"agent"`
	Integration *BundleIntegration `json:"integration,omitempty"`
	APIKey      *BundleAPIKey      `json:"api_key,omitempty"`
	Bundle      *Bundle            `json:"bundle"`
}
