package builtins

import (
	"reflect"
	"sort"
	"testing"
)

func TestAllBuiltinsExposeExpectedCatalog(t *testing.T) {
	expected := map[string][]string{
		"aws":        {"iam.get_role", "iam.list_users", "s3.get_object", "s3.list_buckets"},
		"email":      {"list_inbox", "send"},
		"github":     {"issue.comment", "issue.create", "repo.list", "repo.readme"},
		"postgres":   {"query.readonly"},
		"servicenow": {"incident.create", "incident.get", "incident.list"},
		"webhook":    {"post"},
	}

	gotCatalog := All()
	if len(gotCatalog) != len(expected) {
		t.Fatalf("expected %d builtin connectors, got %d", len(expected), len(gotCatalog))
	}

	gotNames := make([]string, 0, len(gotCatalog))
	for _, connector := range gotCatalog {
		name := connector.Name()
		gotNames = append(gotNames, name)

		wantActions, ok := expected[name]
		if !ok {
			t.Fatalf("unexpected builtin connector %q", name)
		}

		gotActions := append([]string(nil), connector.Actions()...)
		sort.Strings(gotActions)
		wantSorted := append([]string(nil), wantActions...)
		sort.Strings(wantSorted)
		if !reflect.DeepEqual(gotActions, wantSorted) {
			t.Fatalf("connector %q actions mismatch: want %v got %v", name, wantSorted, gotActions)
		}
	}

	sort.Strings(gotNames)
	wantNames := make([]string, 0, len(expected))
	for name := range expected {
		wantNames = append(wantNames, name)
	}
	sort.Strings(wantNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("builtin connector names mismatch: want %v got %v", wantNames, gotNames)
	}
}
