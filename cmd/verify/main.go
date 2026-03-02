package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

type evidenceBundle struct {
	Version    string            `json:"version"`
	TenantID   string            `json:"tenant_id"`
	ExportedAt string            `json:"exported_at"`
	Since      string            `json:"since"`
	Until      string            `json:"until"`
	Events     []json.RawMessage `json:"events"`
	EventCount int               `json:"event_count"`
}

func run(bundlePath string) error {
	if bundlePath == "" {
		return fmt.Errorf("--bundle flag is required")
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		return fmt.Errorf("read bundle file: %w", err)
	}

	var bundle evidenceBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if bundle.Version == "" {
		return fmt.Errorf("missing required field: version")
	}
	if bundle.TenantID == "" {
		return fmt.Errorf("missing required field: tenant_id")
	}
	if bundle.Events == nil {
		return fmt.Errorf("missing required field: events")
	}

	fmt.Printf("Tenant:      %s\n", bundle.TenantID)
	fmt.Printf("Event count: %d\n", len(bundle.Events))
	fmt.Printf("Time range:  %s to %s\n", bundle.Since, bundle.Until)
	fmt.Println("Bundle verification: PASS")
	return nil
}

func main() {
	bundlePath := flag.String("bundle", "", "path to evidence bundle JSON file")
	flag.Parse()

	if err := run(*bundlePath); err != nil {
		fmt.Printf("Bundle verification: FAIL - %s\n", err)
		os.Exit(1)
	}
}
