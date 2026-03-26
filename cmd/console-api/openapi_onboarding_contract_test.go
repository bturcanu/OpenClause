package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOnboardingEndpointsArePresentInOpenAPI(t *testing.T) {
	spec := loadOpenAPISpec(t)
	paths := yamlMap(t, spec["paths"])

	assertRequestSchemaRef(t, paths, "/admin/onboarding/integrations", "post", "#/components/schemas/OnboardingIntegrationInput")
	assertResponseSchemaRef(t, paths, "/admin/onboarding/integrations", "post", "201", "application/json", "#/components/schemas/OnboardingCreatedResponse")
	assertBearerSecurity(t, paths, "/admin/onboarding/integrations", "post")

	assertRequestSchemaRef(t, paths, "/admin/onboarding/bundles/preview", "post", "#/components/schemas/OnboardingPreviewInput")
	assertResponseSchemaRef(t, paths, "/admin/onboarding/bundles/preview", "post", "200", "application/json", "#/components/schemas/OnboardingPreviewResponse")

	assertRequestSchemaRef(t, paths, "/admin/onboarding/bundles/regenerate", "post", "#/components/schemas/OnboardingRegenerateInput")
	assertResponseSchemaRef(t, paths, "/admin/onboarding/bundles/regenerate", "post", "200", "application/json", "#/components/schemas/OnboardingRegeneratedResponse")

	assertRequestSchemaRef(t, paths, "/admin/onboarding/bundles/regenerate-defaults", "post", "#/components/schemas/OnboardingRegenerateDefaultsInput")
	assertResponseSchemaRef(t, paths, "/admin/onboarding/bundles/regenerate-defaults", "post", "200", "application/json", "#/components/schemas/OnboardingRegeneratedDefaultsResponse")

	assertRequestSchemaRef(t, paths, "/admin/onboarding/bundles/archive", "post", "#/components/schemas/OnboardingBundleArchiveRequest")
	archiveSchema := yamlMap(t, yamlMap(t, yamlMap(t, yamlMap(t, yamlMap(t, paths["/admin/onboarding/bundles/archive"])["post"])["responses"])["200"])["content"])
	zipContent := yamlMap(t, archiveSchema["application/zip"])
	zipSchema := yamlMap(t, zipContent["schema"])
	if got := yamlString(t, zipSchema["type"]); got != "string" {
		t.Fatalf("expected archive response type string, got %q", got)
	}
	if got := yamlString(t, zipSchema["format"]); got != "binary" {
		t.Fatalf("expected archive response format binary, got %q", got)
	}

	assertResponseSchemaRef(t, paths, "/admin/tenants/{tenant_id}/agents/{agent_id}/integration", "get", "200", "application/json", "#/components/schemas/OnboardingBundleIntegration")
	assertResponseSchemaRef(t, paths, "/admin/tenants/{tenant_id}/agents/{agent_id}/integration/revisions", "get", "200", "application/json", "#/components/schemas/OnboardingIntegrationRevisionListResponse")
	assertBearerSecurity(t, paths, "/admin/tenants/{tenant_id}/agents/{agent_id}/integration", "get")
	assertBearerSecurity(t, paths, "/admin/tenants/{tenant_id}/agents/{agent_id}/integration/revisions", "get")
	assertBearerSecurity(t, paths, "/admin/tenants/{tenant_id}/agents/{agent_id}/integration/bundle", "get")

	integrationBundleContent := yamlMap(t, yamlMap(t, yamlMap(t, yamlMap(t, yamlMap(t, paths["/admin/tenants/{tenant_id}/agents/{agent_id}/integration/bundle"])["get"])["responses"])["200"])["content"])
	integrationBundleSchema := yamlMap(t, yamlMap(t, integrationBundleContent["application/json"])["schema"])
	oneOfSchemas := yamlSlice(t, integrationBundleSchema["oneOf"])
	if len(oneOfSchemas) != 2 {
		t.Fatalf("expected integration bundle endpoint to expose fetched and fetched_defaults responses, got %+v", integrationBundleSchema)
	}
	assertStringSliceEquals(t, []string{
		yamlString(t, yamlMap(t, oneOfSchemas[0])["$ref"]),
		yamlString(t, yamlMap(t, oneOfSchemas[1])["$ref"]),
	}, []string{
		"#/components/schemas/OnboardingFetchedResponse",
		"#/components/schemas/OnboardingFetchedDefaultsResponse",
	})
}

func TestOnboardingSchemasInOpenAPIMatchShippedContract(t *testing.T) {
	spec := loadOpenAPISpec(t)
	components := yamlMap(t, spec["components"])
	securitySchemes := yamlMap(t, components["securitySchemes"])
	if _, ok := securitySchemes["BearerAuth"]; !ok {
		t.Fatalf("expected BearerAuth security scheme in OpenAPI")
	}

	schemas := yamlMap(t, components["schemas"])
	assertStringSliceEquals(t, yamlStringSlice(t, yamlMap(t, schemas["OnboardingRuntime"])["enum"]), []string{"python", "typescript", "langchain", "openai_local"})
	assertStringSliceEquals(t, yamlStringSlice(t, yamlMap(t, schemas["OnboardingMode"])["enum"]), []string{"created", "preview", "regenerated", "regenerated_defaults", "fetched", "fetched_defaults"})

	integrationInput := yamlMap(t, schemas["OnboardingIntegrationInput"])
	oneOf := yamlSlice(t, integrationInput["oneOf"])
	if len(oneOf) != 2 {
		t.Fatalf("expected create input to use oneOf existing/new tenant shapes, got %+v", integrationInput)
	}

	previewInput := yamlMap(t, schemas["OnboardingPreviewInput"])
	assertStringSliceContains(t, yamlStringSlice(t, previewInput["required"]), []string{"runtime", "tenant_id", "agent_name", "tools"})

	regenerateDefaultsInput := yamlMap(t, schemas["OnboardingRegenerateDefaultsInput"])
	assertStringSliceEquals(t, yamlStringSlice(t, regenerateDefaultsInput["required"]), []string{"tenant_id", "agent_id"})

	bundleResponse := yamlMap(t, schemas["OnboardingBundleResponse"])
	required := yamlStringSlice(t, bundleResponse["required"])
	assertStringSliceContains(t, required, []string{"mode", "tenant", "agent", "bundle"})
	if containsString(required, "api_key") {
		t.Fatalf("expected api_key to stay optional in OnboardingBundleResponse, got %+v", required)
	}
	if containsString(required, "integration") {
		t.Fatalf("expected integration to stay optional in OnboardingBundleResponse, got %+v", required)
	}

	agentSchema := yamlMap(t, schemas["OnboardingBundleAgent"])
	assertStringSliceContains(t, yamlStringSlice(t, agentSchema["required"]), []string{"created_at", "preview"})

	integrationSchema := yamlMap(t, schemas["OnboardingBundleIntegration"])
	assertStringSliceContains(t, yamlStringSlice(t, integrationSchema["required"]), []string{"id", "tenant_id", "agent_id", "runtime", "created_at", "updated_at"})
	integrationProps := yamlMap(t, integrationSchema["properties"])
	if got := yamlString(t, yamlMap(t, integrationProps["runtime"])["$ref"]); got != "#/components/schemas/OnboardingRuntime" {
		t.Fatalf("expected onboarding integration runtime to reuse OnboardingRuntime enum, got %q", got)
	}
	assertStringSliceEquals(t, yamlStringSlice(t, yamlMap(t, integrationProps["approval_posture"])["enum"]), []string{"pilot_safe", "read_only_first", "tenant_default"})

	revisionSchema := yamlMap(t, schemas["OnboardingBundleIntegrationRevision"])
	assertStringSliceContains(t, yamlStringSlice(t, revisionSchema["required"]), []string{"id", "integration_id", "tenant_id", "agent_id", "mode", "runtime", "created_at"})
	assertStringSliceEquals(t, yamlStringSlice(t, yamlMap(t, yamlMap(t, revisionSchema["properties"])["mode"])["enum"]), []string{"created", "regenerated", "regenerated_defaults"})

	apiKeySchema := yamlMap(t, schemas["OnboardingBundleAPIKey"])
	if containsString(yamlStringSlice(t, apiKeySchema["required"]), "raw_key") {
		t.Fatalf("expected raw_key to stay optional in OnboardingBundleAPIKey")
	}
	rawKeyDescription := yamlString(t, yamlMap(t, yamlMap(t, apiKeySchema["properties"])["raw_key"])["description"])
	if !strings.Contains(rawKeyDescription, "Present only for create responses") {
		t.Fatalf("expected raw_key description to capture one-time create behavior, got %q", rawKeyDescription)
	}

	artifactSchema := yamlMap(t, schemas["OnboardingBundleArtifact"])
	artifactProps := yamlMap(t, artifactSchema["properties"])
	if _, ok := artifactProps["executable"]; !ok {
		t.Fatalf("expected onboarding artifact schema to expose executable hints")
	}

	createdResponse := yamlMap(t, schemas["OnboardingCreatedResponse"])
	previewResponse := yamlMap(t, schemas["OnboardingPreviewResponse"])
	regeneratedResponse := yamlMap(t, schemas["OnboardingRegeneratedResponse"])
	defaultsResponse := yamlMap(t, schemas["OnboardingRegeneratedDefaultsResponse"])
	fetchedResponse := yamlMap(t, schemas["OnboardingFetchedResponse"])
	fetchedDefaultsResponse := yamlMap(t, schemas["OnboardingFetchedDefaultsResponse"])
	assertModeEnumOverride(t, createdResponse, "created")
	assertModeEnumOverride(t, previewResponse, "preview")
	assertModeEnumOverride(t, regeneratedResponse, "regenerated")
	assertModeEnumOverride(t, defaultsResponse, "regenerated_defaults")
	assertModeEnumOverride(t, fetchedResponse, "fetched")
	assertModeEnumOverride(t, fetchedDefaultsResponse, "fetched_defaults")
}

func loadOpenAPISpec(t *testing.T) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", "api", "openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal openapi spec: %v", err)
	}
	return doc
}

func assertRequestSchemaRef(t *testing.T, paths map[string]any, path, method, wantRef string) {
	t.Helper()
	operation := yamlMap(t, yamlMap(t, paths[path])[method])
	requestBody := yamlMap(t, operation["requestBody"])
	content := yamlMap(t, requestBody["content"])
	schema := yamlMap(t, yamlMap(t, content["application/json"])["schema"])
	if got := yamlString(t, schema["$ref"]); got != wantRef {
		t.Fatalf("expected %s %s request schema ref %q, got %q", method, path, wantRef, got)
	}
}

func assertResponseSchemaRef(t *testing.T, paths map[string]any, path, method, status, contentType, wantRef string) {
	t.Helper()
	operation := yamlMap(t, yamlMap(t, paths[path])[method])
	responses := yamlMap(t, operation["responses"])
	content := yamlMap(t, yamlMap(t, responses[status])["content"])
	schema := yamlMap(t, yamlMap(t, content[contentType])["schema"])
	if got := yamlString(t, schema["$ref"]); got != wantRef {
		t.Fatalf("expected %s %s %s schema ref %q, got %q", method, path, status, wantRef, got)
	}
}

func assertBearerSecurity(t *testing.T, paths map[string]any, path, method string) {
	t.Helper()
	operation := yamlMap(t, yamlMap(t, paths[path])[method])
	security := yamlSlice(t, operation["security"])
	if len(security) == 0 {
		t.Fatalf("expected %s %s to declare bearer security", method, path)
	}
	first := yamlMap(t, security[0])
	if _, ok := first["BearerAuth"]; !ok {
		t.Fatalf("expected %s %s to use BearerAuth, got %+v", method, path, first)
	}
}

func assertModeEnumOverride(t *testing.T, schema map[string]any, want string) {
	t.Helper()
	allOf := yamlSlice(t, schema["allOf"])
	if len(allOf) != 2 {
		t.Fatalf("expected mode response schema override, got %+v", schema)
	}
	override := yamlMap(t, allOf[1])
	properties := yamlMap(t, override["properties"])
	mode := yamlMap(t, properties["mode"])
	assertStringSliceEquals(t, yamlStringSlice(t, mode["enum"]), []string{want})
}

func assertStringSliceEquals(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func assertStringSliceContains(t *testing.T, haystack, needles []string) {
	t.Helper()
	for _, needle := range needles {
		if !containsString(haystack, needle) {
			t.Fatalf("expected %v to contain %q", haystack, needle)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func yamlMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T (%v)", value, value)
	}
	return m
}

func yamlSlice(t *testing.T, value any) []any {
	t.Helper()
	slice, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T (%v)", value, value)
	}
	return slice
}

func yamlString(t *testing.T, value any) string {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("expected string, got %T (%v)", value, value)
	}
	return text
}

func yamlStringSlice(t *testing.T, value any) []string {
	t.Helper()
	raw := yamlSlice(t, value)
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, yamlString(t, item))
	}
	return out
}
