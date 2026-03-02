package builtins

import (
	"context"

	"github.com/bturcanu/OpenClause/pkg/connectors"
)

// BuiltinConnector is the interface every in-process connector implements.
type BuiltinConnector interface {
	Name() string
	Actions() []string
	Exec(ctx context.Context, req connectors.ExecRequest) connectors.ExecResponse
}

// All returns every built-in connector shipped with OpenClause.
func All() []BuiltinConnector {
	return []BuiltinConnector{
		&GithubConnector{},
		&AWSConnector{},
		&ServiceNowConnector{},
		&EmailConnector{},
		&PostgresConnector{},
		&WebhookConnector{Mock: true},
	}
}
