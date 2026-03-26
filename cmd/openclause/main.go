package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bturcanu/OpenClause/pkg/bridge"
	"github.com/bturcanu/OpenClause/pkg/onboarding"
)

type cliConfig struct {
	baseURL          string
	serverURL        string
	authToken        string
	authProfile      string
	tenantID         string
	newTenantName    string
	tenantName       string
	agentName        string
	agentID          string
	runtime          string
	toolsArg         string
	approvalPosture  string
	environmentLabel string
	ownerName        string
	description      string
	apiKey           string
	outputDir        string
	printOnly        bool
	noFiles          bool
	localOnly        bool
	preview          bool
	regenerate       bool
	useDefaults      bool
}

func main() {
	if err := runWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	return runWithIO(args, os.Stdin, stdout, stderr)
}

func runWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return fmt.Errorf("command required")
	}
	switch args[0] {
	case "init-agent":
		return runInitAgent(args[1:], stdout, stderr)
	case "auth":
		return runAuth(args[1:], stdin, stdout, stderr)
	case "bridge":
		return runBridge(args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInitAgent(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseInitAgentConfig(args, stderr)
	if err != nil {
		return err
	}
	selectedTools, err := parseTools(cfg.toolsArg)
	if err != nil {
		return err
	}
	if len(selectedTools) == 0 && !(cfg.regenerate && cfg.useDefaults && useServerMode(cfg)) {
		return fmt.Errorf("--tools is required")
	}

	if useServerMode(cfg) {
		token, err := resolveCLIAuthToken(cfg)
		if err != nil {
			return err
		}
		cfg.authToken = token
		resp, err := executeServerMode(cfg, selectedTools)
		if err != nil {
			return err
		}
		return emitBundleResult(resp, cfg, stdout)
	}

	bundle, err := buildLocalBundle(cfg, selectedTools)
	if err != nil {
		return err
	}
	resp := &onboarding.BundleResponse{
		Mode: "local",
		Tenant: onboarding.BundleTenant{
			ID:      strings.TrimSpace(cfg.tenantID),
			Name:    resolvedTenantName(cfg),
			Created: false,
		},
		Agent: onboarding.BundleAgent{
			ID:      resolvedAgentID(cfg),
			Name:    strings.TrimSpace(cfg.agentName),
			Status:  "local",
			Preview: false,
		},
		Bundle: bundle,
	}
	return emitBundleResult(resp, cfg, stdout)
}

func runAuth(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printAuthUsage(stderr)
		return fmt.Errorf("auth subcommand required")
	}
	switch args[0] {
	case "login":
		return runAuthLogin(args[1:], stdin, stdout, stderr)
	case "logout":
		return runAuthLogout(args[1:], stdout, stderr)
	case "whoami":
		return runAuthWhoAmI(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printAuthUsage(stdout)
		return nil
	default:
		printAuthUsage(stderr)
		return fmt.Errorf("unknown auth subcommand %q", args[0])
	}
}

type authUser struct {
	ID    string   `json:"id"`
	Email string   `json:"email"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type loginResponse struct {
	Token     string   `json:"token"`
	SessionID string   `json:"session_id,omitempty"`
	User      authUser `json:"user"`
}

func runAuthLogin(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := parseAuthLoginConfig(args, stderr)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(cfg.email)
	if email == "" {
		email, err = promptForValue(stdin, stdout, "Email: ")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(email) == "" {
		return fmt.Errorf("email is required")
	}

	password := cfg.password
	switch {
	case cfg.passwordStdin:
		password, err = readPasswordValue(stdin)
		if err != nil {
			return err
		}
	case strings.TrimSpace(password) == "":
		password, err = promptForValue(stdin, stdout, "Password: ")
		if err != nil {
			return err
		}
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	body, err := json.Marshal(map[string]string{
		"email":    strings.TrimSpace(email),
		"password": password,
	})
	if err != nil {
		return fmt.Errorf("encode auth login payload: %w", err)
	}

	serverURL := strings.TrimRight(strings.TrimSpace(cfg.serverURL), "/")
	req, err := http.NewRequest(http.MethodPost, serverURL+"/auth/login", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build auth login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "openclause-cli/auth-login")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth login request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read auth login response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth login failed: %s", strings.TrimSpace(string(respBody)))
	}

	var out loginResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return fmt.Errorf("decode auth login response: %w", err)
	}
	if strings.TrimSpace(out.Token) == "" {
		return fmt.Errorf("auth login response did not include a token")
	}
	if err := storeAuthProfile(serverURL, cfg.profile, &out); err != nil {
		return err
	}

	profileName := normalizeProfileName(serverURL, cfg.profile)
	fmt.Fprintf(stdout, "Logged in to %s\n", serverURL)
	fmt.Fprintf(stdout, "Profile: %s\n", profileName)
	fmt.Fprintf(stdout, "User: %s (%s)\n", out.User.Email, out.User.ID)
	if len(out.User.Roles) > 0 {
		fmt.Fprintf(stdout, "Roles: %s\n", strings.Join(out.User.Roles, ", "))
	}
	fmt.Fprintln(stdout, "Token stored for future server-backed onboarding commands.")
	return nil
}

func runAuthLogout(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("auth logout", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var serverURL string
	var profile string
	fs.StringVar(&serverURL, "server-url", "", "Console API base URL")
	fs.StringVar(&profile, "profile", "", "Stored auth profile name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadStoredAuthConfig()
	if err != nil {
		return err
	}
	if len(cfg.Profiles) == 0 {
		fmt.Fprintln(stdout, "No stored auth profiles.")
		return nil
	}

	key, err := resolveStoredAuthProfileKey(cfg, strings.TrimSpace(serverURL), strings.TrimSpace(profile))
	if err != nil {
		return err
	}
	delete(cfg.Profiles, key)
	if cfg.CurrentProfile == key {
		cfg.CurrentProfile = ""
		for candidate := range cfg.Profiles {
			cfg.CurrentProfile = candidate
			break
		}
	}
	if err := saveStoredAuthConfig(cfg); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed stored auth profile %s\n", key)
	return nil
}

func runAuthWhoAmI(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("auth whoami", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var serverURL string
	var profile string
	fs.StringVar(&serverURL, "server-url", "", "Console API base URL")
	fs.StringVar(&profile, "profile", "", "Stored auth profile name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadStoredAuthConfig()
	if err != nil {
		return err
	}
	if len(cfg.Profiles) == 0 {
		return fmt.Errorf("no stored auth profiles; run `openclause auth login` first")
	}
	key, err := resolveStoredAuthProfileKey(cfg, strings.TrimSpace(serverURL), strings.TrimSpace(profile))
	if err != nil {
		return err
	}
	entry, ok := cfg.Profiles[key]
	if !ok {
		return fmt.Errorf("stored auth profile %q not found", key)
	}

	fmt.Fprintf(stdout, "Profile: %s\n", key)
	fmt.Fprintf(stdout, "Server: %s\n", entry.ServerURL)
	fmt.Fprintf(stdout, "User: %s (%s)\n", entry.User.Email, entry.User.ID)
	if len(entry.User.Roles) > 0 {
		fmt.Fprintf(stdout, "Roles: %s\n", strings.Join(entry.User.Roles, ", "))
	}
	if strings.TrimSpace(entry.UpdatedAt) != "" {
		fmt.Fprintf(stdout, "Updated: %s\n", entry.UpdatedAt)
	}
	return nil
}

func parseAuthLoginConfig(args []string, stderr io.Writer) (*authCLIConfig, error) {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := &authCLIConfig{}
	fs.StringVar(&cfg.serverURL, "server-url", "http://localhost:8090", "Console API base URL")
	fs.StringVar(&cfg.profile, "profile", "", "Stored auth profile name")
	fs.StringVar(&cfg.email, "email", "", "Console user email")
	fs.StringVar(&cfg.password, "password", "", "Console user password")
	fs.BoolVar(&cfg.passwordStdin, "password-stdin", false, "Read the password from stdin")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if cfg.passwordStdin && strings.TrimSpace(cfg.password) != "" {
		return nil, fmt.Errorf("--password and --password-stdin cannot be combined")
	}
	if strings.TrimSpace(cfg.serverURL) == "" {
		return nil, fmt.Errorf("--server-url is required")
	}
	return cfg, nil
}

func promptForValue(stdin io.Reader, stdout io.Writer, label string) (string, error) {
	fmt.Fprint(stdout, label)
	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read prompt input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func readPasswordValue(stdin io.Reader) (string, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

type bridgeCLIConfig struct {
	configPath string
	profile    string
}

type bridgeDoctorConfig struct {
	configPath string
	profile    string
	json       bool
}

type authCLIConfig struct {
	serverURL     string
	profile       string
	email         string
	password      string
	passwordStdin bool
}

type bridgeChatConfig struct {
	configPath   string
	baseURL      string
	profile      string
	model        string
	systemPrompt string
	prompt       string
}

func runBridge(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printBridgeUsage(stderr)
		return fmt.Errorf("bridge subcommand required")
	}
	switch args[0] {
	case "start":
		return runBridgeStart(args[1:], stdout, stderr)
	case "chat":
		return runBridgeChat(args[1:], stdin, stdout, stderr)
	case "doctor":
		return runBridgeDoctor(args[1:], stdout, stderr)
	case "mcp":
		return runBridgeMCP(args[1:], stdin, stdout, stderr)
	case "-h", "--help", "help":
		printBridgeUsage(stdout)
		return nil
	default:
		printBridgeUsage(stderr)
		return fmt.Errorf("unknown bridge subcommand %q", args[0])
	}
}

func runBridgeStart(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseBridgeStartConfig(args, stderr)
	if err != nil {
		return err
	}
	resolved, err := bridge.LoadConfigFile(cfg.configPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Starting OpenClause bridge on %s\n", resolved.Listen)
	fmt.Fprintf(stdout, "Tenant: %s\n", resolved.TenantID)
	fmt.Fprintf(stdout, "Agent: %s\n", resolved.AgentID)
	fmt.Fprintf(stdout, "Configured tools: %d\n", len(resolved.Tools))
	if resolved.OpenAI.Enabled {
		fmt.Fprintf(stdout, "Chat endpoint: %s\n", bridgeLocalBaseURL(resolved.Listen)+"/v1")
		fmt.Fprintf(stdout, "Default model: %s\n", resolved.OpenAI.Model)
		fmt.Fprintln(stdout, "Next: run `python local_model_agent.py --smoke` for a first call or `openclause bridge chat --config ./openclause-bridge.yaml` for an interactive chat.")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return bridge.Serve(ctx, resolved, logger)
}

func parseBridgeStartConfig(args []string, stderr io.Writer) (*bridgeCLIConfig, error) {
	fs := flag.NewFlagSet("bridge start", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := &bridgeCLIConfig{}
	fs.StringVar(&cfg.configPath, "config", "", "Path to the local bridge YAML config")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.configPath) == "" {
		return nil, fmt.Errorf("--config is required")
	}
	return cfg, nil
}

func runBridgeMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := parseBridgeMCPConfig(args, stderr)
	if err != nil {
		return err
	}
	resolved, err := bridge.LoadConfigFile(cfg.configPath)
	if err != nil {
		return err
	}
	if name := strings.TrimSpace(cfg.profile); name != "" {
		selectedProfile, ok := resolved.ResolveProfile(name)
		if !ok || selectedProfile == nil {
			return fmt.Errorf("bridge profile %q not found", name)
		}
		resolved.DefaultProfile = selectedProfile.Name
		resolved.BaseURL = selectedProfile.BaseURL
		resolved.TenantID = selectedProfile.TenantID
		resolved.AgentID = selectedProfile.AgentID
		resolved.APIKey = selectedProfile.APIKey
		resolved.Defaults = selectedProfile.Defaults
		resolved.Tools = append([]bridge.ToolConfig(nil), selectedProfile.Tools...)
		resolved.OpenAI = selectedProfile.OpenAI
	}
	server, err := bridge.NewServer(resolved, slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return server.ServeMCPStdio(ctx, stdin, stdout)
}

func parseBridgeMCPConfig(args []string, stderr io.Writer) (*bridgeCLIConfig, error) {
	fs := flag.NewFlagSet("bridge mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := &bridgeCLIConfig{}
	fs.StringVar(&cfg.configPath, "config", "", "Path to the local bridge YAML config")
	fs.StringVar(&cfg.profile, "profile", "", "Named bridge profile when the bridge config exposes multiple profiles")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.configPath) == "" {
		return nil, fmt.Errorf("--config is required")
	}
	return cfg, nil
}

type bridgeDoctorCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Target  string `json:"target,omitempty"`
	Message string `json:"message"`
}

type bridgeDoctorReport struct {
	Status  string              `json:"status"`
	Profile string              `json:"profile"`
	Tenant  string              `json:"tenant_id"`
	Agent   string              `json:"agent_id"`
	Checks  []bridgeDoctorCheck `json:"checks"`
}

func runBridgeDoctor(args []string, stdout, stderr io.Writer) error {
	cfg, err := parseBridgeDoctorConfig(args, stderr)
	if err != nil {
		return err
	}
	report, err := inspectBridgeConfig(cfg)
	if report != nil {
		if printErr := printBridgeDoctorReport(stdout, report, cfg.json); printErr != nil {
			return printErr
		}
	}
	if err != nil {
		return err
	}
	if report.Status == "fail" {
		return fmt.Errorf("bridge doctor found one or more failures")
	}
	return nil
}

func parseBridgeDoctorConfig(args []string, stderr io.Writer) (*bridgeDoctorConfig, error) {
	fs := flag.NewFlagSet("bridge doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := &bridgeDoctorConfig{}
	fs.StringVar(&cfg.configPath, "config", "", "Path to the local bridge YAML config")
	fs.StringVar(&cfg.profile, "profile", "", "Named bridge profile to validate")
	fs.BoolVar(&cfg.json, "json", false, "Emit machine-readable JSON output")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.configPath) == "" {
		return nil, fmt.Errorf("--config is required")
	}
	return cfg, nil
}

func inspectBridgeConfig(cfg *bridgeDoctorConfig) (*bridgeDoctorReport, error) {
	if cfg == nil {
		return nil, fmt.Errorf("bridge doctor config required")
	}
	resolved, err := bridge.LoadConfigFile(strings.TrimSpace(cfg.configPath))
	if err != nil {
		return nil, err
	}
	profile, ok := resolved.ResolveProfile(strings.TrimSpace(cfg.profile))
	if !ok || profile == nil {
		name := strings.TrimSpace(cfg.profile)
		if name == "" {
			name = resolved.DefaultProfile
		}
		return nil, fmt.Errorf("bridge profile %q not found", name)
	}

	report := &bridgeDoctorReport{
		Status:  "ok",
		Profile: profile.Name,
		Tenant:  profile.TenantID,
		Agent:   profile.AgentID,
		Checks: []bridgeDoctorCheck{{
			ID:      "config",
			Status:  "ok",
			Target:  cfg.configPath,
			Message: fmt.Sprintf("Loaded profile %s for tenant %s and agent %s.", profile.Name, profile.TenantID, profile.AgentID),
		}},
	}
	addCheck := func(check bridgeDoctorCheck) {
		report.Checks = append(report.Checks, check)
		switch check.Status {
		case "fail":
			report.Status = "fail"
		case "warn":
			if report.Status != "fail" {
				report.Status = "warn"
			}
		}
	}

	addCheck(probeGatewayHealth(profile))
	addCheck(probeGatewayConnectors(profile))
	addCheck(probeGatewayAuth(profile))
	addCheck(probeBridgeToolsSurface(resolved, profile.Name))
	addCheck(probeBridgeMCPSurface(resolved, profile.Name))
	if profile.OpenAI.Enabled {
		modelCheck, models, err := probeOpenAIModels(profile)
		addCheck(modelCheck)
		addCheck(probeBridgeModelsSurface(resolved, profile.Name))
		if err == nil {
			addCheck(checkConfiguredModel(models, profile))
		}
	} else {
		addCheck(bridgeDoctorCheck{
			ID:      "openai.config",
			Status:  "warn",
			Message: "OpenAI upstream is not configured for this profile, so chat and native LM Studio model checks were skipped.",
		})
	}

	return report, nil
}

func printBridgeDoctorReport(w io.Writer, report *bridgeDoctorReport, asJSON bool) error {
	if report == nil {
		return fmt.Errorf("bridge doctor report required")
	}
	if asJSON {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("encode bridge doctor report: %w", err)
		}
		_, err = fmt.Fprintln(w, string(encoded))
		return err
	}

	fmt.Fprintf(w, "Bridge doctor for profile %s (%s / %s)\n", report.Profile, report.Tenant, report.Agent)
	for _, check := range report.Checks {
		fmt.Fprintf(w, "[%s] %s", strings.ToUpper(check.Status), check.ID)
		if strings.TrimSpace(check.Target) != "" {
			fmt.Fprintf(w, " (%s)", check.Target)
		}
		fmt.Fprintf(w, ": %s\n", check.Message)
	}
	fmt.Fprintf(w, "Overall status: %s\n", strings.ToUpper(report.Status))
	if report.Status == "ok" {
		fmt.Fprintln(w, "Next: start the bridge and connect your local runtime or MCP client.")
	}
	return nil
}

func probeGatewayHealth(profile *bridge.ResolvedProfile) bridgeDoctorCheck {
	target := strings.TrimRight(profile.BaseURL, "/") + "/healthz"
	resp, err := http.Get(target)
	if err != nil {
		return bridgeDoctorCheck{
			ID:      "gateway.health",
			Status:  "fail",
			Target:  target,
			Message: fmt.Sprintf("Gateway was not reachable: %v", err),
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return bridgeDoctorCheck{
			ID:      "gateway.health",
			Status:  "fail",
			Target:  target,
			Message: fmt.Sprintf("Gateway health check returned %d.", resp.StatusCode),
		}
	}
	return bridgeDoctorCheck{
		ID:      "gateway.health",
		Status:  "ok",
		Target:  target,
		Message: "Gateway responded to /healthz.",
	}
}

func probeGatewayConnectors(profile *bridge.ResolvedProfile) bridgeDoctorCheck {
	target := strings.TrimRight(profile.BaseURL, "/") + "/v1/connectors"
	resp, err := http.Get(target)
	if err != nil {
		return bridgeDoctorCheck{
			ID:      "gateway.connectors",
			Status:  "warn",
			Target:  target,
			Message: fmt.Sprintf("Connector catalog could not be loaded: %v", err),
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return bridgeDoctorCheck{
			ID:      "gateway.connectors",
			Status:  "warn",
			Target:  target,
			Message: fmt.Sprintf("Connector catalog returned %d. The bridge can still run, but onboarding/runtime tool guidance may be incomplete.", resp.StatusCode),
		}
	}
	return bridgeDoctorCheck{
		ID:      "gateway.connectors",
		Status:  "ok",
		Target:  target,
		Message: "Connector catalog was reachable.",
	}
}

func probeGatewayAuth(profile *bridge.ResolvedProfile) bridgeDoctorCheck {
	target := strings.TrimRight(profile.BaseURL, "/") + "/v1/toolcalls"
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(`{}`))
	if err != nil {
		return bridgeDoctorCheck{
			ID:      "gateway.auth",
			Status:  "fail",
			Target:  target,
			Message: fmt.Sprintf("Could not build gateway auth probe: %v", err),
		}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", profile.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return bridgeDoctorCheck{
			ID:      "gateway.auth",
			Status:  "fail",
			Target:  target,
			Message: fmt.Sprintf("Gateway auth probe failed: %v", err),
		}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return bridgeDoctorCheck{
			ID:      "gateway.auth",
			Status:  "fail",
			Target:  target,
			Message: "Gateway rejected the configured API key. Rotate the key or regenerate the bundle.",
		}
	case http.StatusBadRequest, http.StatusUnprocessableEntity, http.StatusMethodNotAllowed, http.StatusOK:
		return bridgeDoctorCheck{
			ID:      "gateway.auth",
			Status:  "ok",
			Target:  target,
			Message: "Gateway accepted the API key probe. The body was intentionally invalid, so a non-auth error here is expected.",
		}
	default:
		return bridgeDoctorCheck{
			ID:      "gateway.auth",
			Status:  "warn",
			Target:  target,
			Message: fmt.Sprintf("Gateway returned %d to the auth probe. Check the gateway logs if first tool calls still fail.", resp.StatusCode),
		}
	}
}

func probeBridgeToolsSurface(resolved *bridge.ResolvedConfig, profileName string) bridgeDoctorCheck {
	server, err := bridge.NewServer(resolved, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return bridgeDoctorCheck{ID: "bridge.tools", Status: "fail", Message: fmt.Sprintf("Bridge server could not be initialized: %v", err)}
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/bridge/tools?profile="+profileName, http.NoBody)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return bridgeDoctorCheck{ID: "bridge.tools", Status: "fail", Message: fmt.Sprintf("Bridge tools surface returned %d.", rec.Code)}
	}
	return bridgeDoctorCheck{
		ID:      "bridge.tools",
		Status:  "ok",
		Message: "Bridge tools surface validated successfully.",
	}
}

func probeBridgeModelsSurface(resolved *bridge.ResolvedConfig, profileName string) bridgeDoctorCheck {
	server, err := bridge.NewServer(resolved, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return bridgeDoctorCheck{ID: "bridge.models", Status: "fail", Message: fmt.Sprintf("Bridge server could not be initialized: %v", err)}
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/models?profile="+profileName, http.NoBody)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return bridgeDoctorCheck{ID: "bridge.models", Status: "fail", Message: fmt.Sprintf("Bridge OpenAI models proxy returned %d.", rec.Code)}
	}
	return bridgeDoctorCheck{
		ID:      "bridge.models",
		Status:  "ok",
		Message: "Bridge OpenAI models proxy can reach the configured upstream.",
	}
}

func probeBridgeMCPSurface(resolved *bridge.ResolvedConfig, profileName string) bridgeDoctorCheck {
	server, err := bridge.NewServer(resolved, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return bridgeDoctorCheck{ID: "bridge.mcp", Status: "fail", Message: fmt.Sprintf("Bridge server could not be initialized: %v", err)}
	}
	initReq := httptest.NewRequest(http.MethodPost, "/mcp?profile="+profileName, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	initReq.Header.Set("Content-Type", "application/json")
	initRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		return bridgeDoctorCheck{ID: "bridge.mcp", Status: "fail", Message: fmt.Sprintf("Bridge MCP initialize returned %d.", initRec.Code)}
	}
	sessionID := strings.TrimSpace(initRec.Header().Get("Mcp-Session-Id"))
	if sessionID == "" {
		return bridgeDoctorCheck{ID: "bridge.mcp", Status: "fail", Message: "Bridge MCP initialize did not return a session id."}
	}
	listReq := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`))
	listReq.Header.Set("Content-Type", "application/json")
	listReq.Header.Set("Mcp-Session-Id", sessionID)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		return bridgeDoctorCheck{ID: "bridge.mcp", Status: "fail", Message: fmt.Sprintf("Bridge MCP tools/list returned %d.", listRec.Code)}
	}
	return bridgeDoctorCheck{
		ID:      "bridge.mcp",
		Status:  "ok",
		Message: "Bridge MCP initialize and tools/list succeeded.",
	}
}

func probeOpenAIModels(profile *bridge.ResolvedProfile) (bridgeDoctorCheck, []string, error) {
	target := strings.TrimRight(profile.OpenAI.UpstreamBaseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, target, http.NoBody)
	if err != nil {
		return bridgeDoctorCheck{ID: "openai.models", Status: "fail", Target: target, Message: fmt.Sprintf("Could not build upstream model probe: %v", err)}, nil, err
	}
	if strings.TrimSpace(profile.OpenAI.UpstreamAPIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(profile.OpenAI.UpstreamAPIKey))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		check := bridgeDoctorCheck{
			ID:      "openai.models",
			Status:  "fail",
			Target:  target,
			Message: fmt.Sprintf("Upstream OpenAI-compatible server was not reachable: %v", err),
		}
		return check, nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		check := bridgeDoctorCheck{
			ID:      "openai.models",
			Status:  "fail",
			Target:  target,
			Message: fmt.Sprintf("Could not read upstream models response: %v", readErr),
		}
		return check, nil, readErr
	}
	if resp.StatusCode != http.StatusOK {
		check := bridgeDoctorCheck{
			ID:      "openai.models",
			Status:  "fail",
			Target:  target,
			Message: fmt.Sprintf("Upstream OpenAI-compatible server returned %d.", resp.StatusCode),
		}
		return check, nil, fmt.Errorf("%s", check.Message)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		check := bridgeDoctorCheck{
			ID:      "openai.models",
			Status:  "fail",
			Target:  target,
			Message: fmt.Sprintf("Could not decode upstream models response: %v", err),
		}
		return check, nil, err
	}
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.ID) != "" {
			models = append(models, strings.TrimSpace(item.ID))
		}
	}
	return bridgeDoctorCheck{
		ID:      "openai.models",
		Status:  "ok",
		Target:  target,
		Message: fmt.Sprintf("Upstream OpenAI-compatible server returned %d models.", len(models)),
	}, models, nil
}

func checkConfiguredModel(models []string, profile *bridge.ResolvedProfile) bridgeDoctorCheck {
	configured := strings.TrimSpace(profile.OpenAI.Model)
	for _, item := range models {
		if strings.EqualFold(item, configured) {
			return bridgeDoctorCheck{
				ID:      "openai.model",
				Status:  "ok",
				Target:  configured,
				Message: "Configured model is available from the upstream server.",
			}
		}
	}
	return bridgeDoctorCheck{
		ID:      "openai.model",
		Status:  "fail",
		Target:  configured,
		Message: "Configured model was not present in the upstream model list. Update the bridge config or start the model in LM Studio.",
	}
}

func runBridgeChat(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := parseBridgeChatConfig(args, stderr)
	if err != nil {
		return err
	}
	baseURL, model, systemPrompt, err := resolveBridgeChatTarget(cfg)
	if err != nil {
		return err
	}

	if strings.TrimSpace(cfg.prompt) != "" {
		text, err := requestBridgeChat(baseURL, cfg.profile, model, initialChatMessages(systemPrompt, cfg.prompt))
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, text)
		return nil
	}

	fmt.Fprintf(stdout, "Connected to %s using model %s\n", baseURL, model)
	fmt.Fprintln(stdout, "Type a prompt and press enter. Commands: /help, /reset, /exit")

	messages := initialChatConversation(systemPrompt)
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	for {
		fmt.Fprint(stdout, "You> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read chat input: %w", err)
			}
			fmt.Fprintln(stdout)
			return nil
		}
		line := strings.TrimSpace(scanner.Text())
		switch line {
		case "":
			continue
		case "/exit", "/quit":
			fmt.Fprintln(stdout, "Bye.")
			return nil
		case "/help":
			fmt.Fprintln(stdout, "Type a normal chat prompt to send it through the local bridge. Use /reset to clear the current conversation or /exit to leave.")
			continue
		case "/reset":
			messages = initialChatConversation(systemPrompt)
			fmt.Fprintln(stdout, "Conversation reset.")
			continue
		}

		nextMessages := append(append([]bridgeChatMessage{}, messages...), bridgeChatMessage{
			Role:    "user",
			Content: line,
		})
		text, err := requestBridgeChat(baseURL, cfg.profile, model, nextMessages)
		if err != nil {
			fmt.Fprintf(stderr, "chat request failed: %v\n", err)
			continue
		}

		fmt.Fprintf(stdout, "Assistant> %s\n", text)
		messages = append(nextMessages, bridgeChatMessage{
			Role:    "assistant",
			Content: text,
		})
	}
}

func parseBridgeChatConfig(args []string, stderr io.Writer) (*bridgeChatConfig, error) {
	fs := flag.NewFlagSet("bridge chat", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := &bridgeChatConfig{}
	fs.StringVar(&cfg.configPath, "config", "", "Path to the local bridge YAML config")
	fs.StringVar(&cfg.baseURL, "base-url", "", "Bridge base URL, with or without /v1")
	fs.StringVar(&cfg.profile, "profile", "", "Named bridge profile when the bridge config exposes multiple profiles")
	fs.StringVar(&cfg.model, "model", "", "OpenAI-compatible model id to send")
	fs.StringVar(&cfg.systemPrompt, "system", "", "Optional extra system prompt to prepend for this chat session")
	fs.StringVar(&cfg.prompt, "prompt", "", "Optional one-shot prompt; omit for interactive mode")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.configPath) == "" && strings.TrimSpace(cfg.baseURL) == "" {
		return nil, fmt.Errorf("--config or --base-url is required")
	}
	return cfg, nil
}

func resolveBridgeChatTarget(cfg *bridgeChatConfig) (string, string, string, error) {
	if cfg == nil {
		return "", "", "", fmt.Errorf("bridge chat config required")
	}

	baseURL := strings.TrimSpace(cfg.baseURL)
	model := strings.TrimSpace(cfg.model)
	systemPrompt := strings.TrimSpace(cfg.systemPrompt)

	if strings.TrimSpace(cfg.configPath) != "" {
		resolved, err := bridge.LoadConfigFile(strings.TrimSpace(cfg.configPath))
		if err != nil {
			return "", "", "", err
		}
		selectedProfile, ok := resolved.ResolveProfile(strings.TrimSpace(cfg.profile))
		if !ok || selectedProfile == nil {
			name := strings.TrimSpace(cfg.profile)
			if name == "" {
				name = resolved.DefaultProfile
			}
			return "", "", "", fmt.Errorf("bridge profile %q not found", name)
		}
		if baseURL == "" {
			baseURL = bridgeLocalBaseURL(resolved.Listen)
		}
		if model == "" {
			model = strings.TrimSpace(selectedProfile.OpenAI.Model)
		}
		if systemPrompt == "" {
			systemPrompt = strings.TrimSpace(selectedProfile.OpenAI.SystemPrompt)
		}
	}

	if baseURL == "" {
		return "", "", "", fmt.Errorf("--base-url is required when --config does not provide a listen address")
	}
	if model == "" {
		return "", "", "", fmt.Errorf("--model is required when the bridge config does not include an openai model")
	}
	return normalizeBridgeAPIBaseURL(baseURL), model, systemPrompt, nil
}

func bridgeLocalBaseURL(listen string) string {
	trimmed := strings.TrimSpace(strings.TrimRight(listen, "/"))
	if trimmed == "" {
		trimmed = bridge.DefaultListenAddr
	}
	if strings.Contains(trimmed, "://") {
		return strings.TrimRight(trimmed, "/")
	}
	if strings.HasPrefix(trimmed, ":") {
		return "http://127.0.0.1" + trimmed
	}
	if strings.HasPrefix(trimmed, "0.0.0.0:") {
		return "http://127.0.0.1:" + strings.TrimPrefix(trimmed, "0.0.0.0:")
	}
	return "http://" + trimmed
}

func normalizeBridgeAPIBaseURL(base string) string {
	trimmed := strings.TrimSpace(strings.TrimRight(base, "/"))
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed
	}
	return trimmed + "/v1"
}

func parseInitAgentConfig(args []string, stderr io.Writer) (*cliConfig, error) {
	fs := flag.NewFlagSet("init-agent", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := &cliConfig{}
	fs.StringVar(&cfg.baseURL, "base-url", "http://localhost:8080", "OpenClause gateway base URL for generated artifacts")
	fs.StringVar(&cfg.serverURL, "server-url", "", "Console API base URL for server-backed create")
	fs.StringVar(&cfg.authToken, "auth-token", "", "Console API bearer token for server-backed create")
	fs.StringVar(&cfg.authProfile, "auth-profile", "", "Stored CLI auth profile name for server-backed flows")
	fs.StringVar(&cfg.tenantID, "tenant-id", "", "Existing tenant ID")
	fs.StringVar(&cfg.newTenantName, "new-tenant-name", "", "Create a tenant inline through the server-backed flow")
	fs.StringVar(&cfg.tenantName, "tenant-name", "", "Tenant display name used in generated README copy")
	fs.StringVar(&cfg.agentName, "agent-name", "", "Agent name")
	fs.StringVar(&cfg.agentID, "agent-id", "", "Existing or planned agent ID for local-only generation")
	fs.StringVar(&cfg.runtime, "runtime", string(onboarding.RuntimePython), "Runtime: python|typescript|langchain|openai_local")
	fs.StringVar(&cfg.toolsArg, "tools", "", "Comma-separated governed tools in tool:action form")
	fs.StringVar(&cfg.approvalPosture, "approval-posture", "pilot_safe", "Approval posture hint")
	fs.StringVar(&cfg.environmentLabel, "environment-label", "dev", "Environment label")
	fs.StringVar(&cfg.ownerName, "owner-name", "", "Owner or team hint")
	fs.StringVar(&cfg.description, "description", "", "Optional integration description")
	fs.StringVar(&cfg.apiKey, "api-key", "", "Optional raw API key to embed in local-only generated env files")
	fs.StringVar(&cfg.outputDir, "output-dir", ".", "Directory for generated artifacts")
	fs.BoolVar(&cfg.printOnly, "print-only", false, "Print the generated bundle instead of writing files")
	fs.BoolVar(&cfg.noFiles, "no-files", false, "Generate the bundle summary without writing files")
	fs.BoolVar(&cfg.localOnly, "local-only", false, "Skip server-backed create and generate artifacts locally only")
	fs.BoolVar(&cfg.preview, "preview", false, "Call the server-backed non-destructive onboarding preview flow")
	fs.BoolVar(&cfg.regenerate, "regenerate", false, "Call the server-backed bundle regeneration flow for an existing agent")
	fs.BoolVar(&cfg.useDefaults, "use-defaults", false, "Use explicit server-side onboarding defaults when regenerating an existing tenant + agent bundle")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if cfg.preview && cfg.regenerate {
		return nil, fmt.Errorf("--preview and --regenerate cannot be combined")
	}
	if cfg.printOnly && cfg.noFiles {
		return nil, fmt.Errorf("--print-only and --no-files cannot be combined")
	}
	if cfg.useDefaults && !cfg.regenerate {
		return nil, fmt.Errorf("--use-defaults can only be used with --regenerate")
	}
	if strings.TrimSpace(cfg.agentName) == "" && !cfg.regenerate {
		return nil, fmt.Errorf("--agent-name is required")
	}
	if cfg.localOnly && (cfg.preview || cfg.regenerate) {
		return nil, fmt.Errorf("--local-only cannot be combined with --preview or --regenerate")
	}
	if cfg.localOnly && strings.TrimSpace(cfg.newTenantName) != "" {
		return nil, fmt.Errorf("--new-tenant-name is not supported with --local-only; use --tenant-id")
	}
	if cfg.preview {
		if strings.TrimSpace(cfg.tenantID) == "" {
			return nil, fmt.Errorf("--tenant-id is required for --preview")
		}
		if strings.TrimSpace(cfg.newTenantName) != "" {
			return nil, fmt.Errorf("--new-tenant-name is not supported for --preview")
		}
	}
	if cfg.regenerate {
		if strings.TrimSpace(cfg.tenantID) == "" {
			return nil, fmt.Errorf("--tenant-id is required for --regenerate")
		}
		if strings.TrimSpace(cfg.agentID) == "" {
			return nil, fmt.Errorf("--agent-id is required for --regenerate")
		}
		if strings.TrimSpace(cfg.newTenantName) != "" {
			return nil, fmt.Errorf("--new-tenant-name is not supported for --regenerate")
		}
	}
	if strings.TrimSpace(cfg.tenantID) == "" && strings.TrimSpace(cfg.newTenantName) == "" {
		return nil, fmt.Errorf("--tenant-id or --new-tenant-name is required")
	}
	return cfg, nil
}

func useServerMode(cfg *cliConfig) bool {
	return strings.TrimSpace(cfg.serverURL) != "" && !cfg.localOnly
}

func resolveCLIAuthToken(cfg *cliConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("cli config required")
	}
	if token := strings.TrimSpace(cfg.authToken); token != "" {
		return token, nil
	}
	token, err := resolveStoredAuthToken(strings.TrimSpace(cfg.serverURL), strings.TrimSpace(cfg.authProfile))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token) != "" {
		return token, nil
	}
	if token := strings.TrimSpace(os.Getenv("OPENCLAUSE_AUTH_TOKEN")); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("--auth-token is required when using --server-url for server-backed preview, create, or regenerate; or run `openclause auth login`")
}

func executeServerMode(cfg *cliConfig, selectedTools []onboarding.SelectedTool) (*onboarding.BundleResponse, error) {
	switch {
	case cfg.preview:
		return previewBundleViaServer(cfg, selectedTools)
	case cfg.regenerate:
		if cfg.useDefaults {
			return regenerateBundleWithDefaultsViaServer(cfg)
		}
		return regenerateBundleViaServer(cfg, selectedTools)
	default:
		return createIntegrationViaServer(cfg, selectedTools)
	}
}

func createIntegrationViaServer(cfg *cliConfig, selectedTools []onboarding.SelectedTool) (*onboarding.BundleResponse, error) {
	payload := onboarding.IntegrationInput{
		Runtime:          strings.TrimSpace(cfg.runtime),
		TenantID:         strings.TrimSpace(cfg.tenantID),
		NewTenantName:    strings.TrimSpace(cfg.newTenantName),
		AgentName:        strings.TrimSpace(cfg.agentName),
		EnvironmentLabel: strings.TrimSpace(cfg.environmentLabel),
		OwnerName:        strings.TrimSpace(cfg.ownerName),
		Description:      strings.TrimSpace(cfg.description),
		ApprovalPosture:  strings.TrimSpace(cfg.approvalPosture),
		Tools:            selectedTools,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode create payload: %w", err)
	}

	base := strings.TrimRight(strings.TrimSpace(cfg.serverURL), "/")
	req, err := http.NewRequest(http.MethodPost, base+"/admin/onboarding/integrations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.authToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create integration request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read create response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create integration failed: %s", strings.TrimSpace(string(respBody)))
	}

	var out onboarding.BundleResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode create response: %w", err)
	}
	return &out, nil
}

func previewBundleViaServer(cfg *cliConfig, selectedTools []onboarding.SelectedTool) (*onboarding.BundleResponse, error) {
	payload := onboarding.IntegrationInput{
		Runtime:          strings.TrimSpace(cfg.runtime),
		TenantID:         strings.TrimSpace(cfg.tenantID),
		AgentName:        strings.TrimSpace(cfg.agentName),
		EnvironmentLabel: strings.TrimSpace(cfg.environmentLabel),
		OwnerName:        strings.TrimSpace(cfg.ownerName),
		Description:      strings.TrimSpace(cfg.description),
		ApprovalPosture:  strings.TrimSpace(cfg.approvalPosture),
		Tools:            selectedTools,
	}
	return postServerBundle(cfg, "/admin/onboarding/bundles/preview", http.StatusOK, payload, "preview bundle")
}

func regenerateBundleViaServer(cfg *cliConfig, selectedTools []onboarding.SelectedTool) (*onboarding.BundleResponse, error) {
	payload := onboarding.RegenerateInput{
		IntegrationInput: onboarding.IntegrationInput{
			Runtime:          strings.TrimSpace(cfg.runtime),
			TenantID:         strings.TrimSpace(cfg.tenantID),
			AgentName:        strings.TrimSpace(cfg.agentName),
			EnvironmentLabel: strings.TrimSpace(cfg.environmentLabel),
			OwnerName:        strings.TrimSpace(cfg.ownerName),
			Description:      strings.TrimSpace(cfg.description),
			ApprovalPosture:  strings.TrimSpace(cfg.approvalPosture),
			Tools:            selectedTools,
		},
		AgentID: strings.TrimSpace(cfg.agentID),
	}
	return postServerBundle(cfg, "/admin/onboarding/bundles/regenerate", http.StatusOK, payload, "regenerate bundle")
}

func regenerateBundleWithDefaultsViaServer(cfg *cliConfig) (*onboarding.BundleResponse, error) {
	payload := onboarding.RegenerateInput{
		IntegrationInput: onboarding.IntegrationInput{
			TenantID:         strings.TrimSpace(cfg.tenantID),
			EnvironmentLabel: strings.TrimSpace(cfg.environmentLabel),
			OwnerName:        strings.TrimSpace(cfg.ownerName),
			Description:      strings.TrimSpace(cfg.description),
		},
		AgentID: strings.TrimSpace(cfg.agentID),
	}
	return postServerBundle(cfg, "/admin/onboarding/bundles/regenerate-defaults", http.StatusOK, payload, "regenerate bundle with defaults")
}

func postServerBundle(cfg *cliConfig, path string, expectedStatus int, payload any, action string) (*onboarding.BundleResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode %s payload: %w", action, err)
	}

	base := strings.TrimRight(strings.TrimSpace(cfg.serverURL), "/")
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", action, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.authToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", action, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", action, err)
	}
	if resp.StatusCode != expectedStatus {
		return nil, fmt.Errorf("%s failed: %s", action, strings.TrimSpace(string(respBody)))
	}

	var out onboarding.BundleResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", action, err)
	}
	return &out, nil
}

func buildLocalBundle(cfg *cliConfig, selectedTools []onboarding.SelectedTool) (*onboarding.Bundle, error) {
	resolvedRuntime := onboarding.Runtime(strings.TrimSpace(cfg.runtime))
	if resolvedRuntime != onboarding.RuntimePython && resolvedRuntime != onboarding.RuntimeTypeScript && resolvedRuntime != onboarding.RuntimeLangChain && resolvedRuntime != onboarding.RuntimeOpenAILocal {
		return nil, fmt.Errorf("--runtime must be one of: python, typescript, langchain, openai_local")
	}

	if strings.TrimSpace(cfg.tenantID) == "" {
		return nil, fmt.Errorf("--tenant-id is required for local-only generation")
	}

	apiKeyValue := strings.TrimSpace(cfg.apiKey)
	apiKeyMode := onboarding.APIKeyModeExistingKeyRef
	if apiKeyValue == "" {
		apiKeyValue = "${OPENCLAUSE_API_KEY:-set-me}"
	} else {
		apiKeyMode = onboarding.APIKeyModeRawProvided
	}

	return onboarding.BuildBundle(onboarding.BundleRequest{
		BaseURL:          strings.TrimSpace(cfg.baseURL),
		TenantID:         strings.TrimSpace(cfg.tenantID),
		TenantName:       resolvedTenantName(cfg),
		AgentID:          resolvedAgentID(cfg),
		AgentName:        strings.TrimSpace(cfg.agentName),
		APIKey:           apiKeyValue,
		APIKeyMode:       apiKeyMode,
		Runtime:          resolvedRuntime,
		ApprovalPosture:  strings.TrimSpace(cfg.approvalPosture),
		EnvironmentLabel: strings.TrimSpace(cfg.environmentLabel),
		OwnerName:        strings.TrimSpace(cfg.ownerName),
		Description:      strings.TrimSpace(cfg.description),
		Tools:            selectedTools,
	})
}

func emitBundleResult(resp *onboarding.BundleResponse, cfg *cliConfig, stdout io.Writer) error {
	if resp == nil || resp.Bundle == nil {
		return fmt.Errorf("bundle response missing bundle")
	}

	fmt.Fprintf(stdout, "%s\n", resp.Bundle.Title)
	fmt.Fprintf(stdout, "Mode: %s\n", humanMode(resp.Mode))
	fmt.Fprintf(stdout, "Tenant: %s (%s)\n", resp.Tenant.Name, resp.Tenant.ID)
	fmt.Fprintf(stdout, "Agent: %s (%s)\n", resp.Agent.Name, resp.Agent.ID)
	fmt.Fprintf(stdout, "Runtime: %s\n", resp.Bundle.RuntimeLabel)
	fmt.Fprintf(stdout, "Artifacts: %d\n", len(resp.Bundle.Artifacts))
	if len(resp.Bundle.AppliedDefaults) > 0 {
		fmt.Fprintln(stdout, "Defaults applied:")
		for _, item := range resp.Bundle.AppliedDefaults {
			if strings.TrimSpace(item.Reason) != "" {
				fmt.Fprintf(stdout, "- %s=%s (%s)\n", item.Field, item.Value, item.Reason)
			} else {
				fmt.Fprintf(stdout, "- %s=%s\n", item.Field, item.Value)
			}
		}
	}

	if cfg.printOnly {
		if resp.APIKey != nil && strings.TrimSpace(resp.APIKey.RawKey) != "" {
			fmt.Fprintf(stdout, "One-time API key: %s\n", resp.APIKey.RawKey)
		} else if resp.APIKey != nil && strings.TrimSpace(resp.APIKey.KeyPrefix) != "" {
			fmt.Fprintf(stdout, "Existing API key reference: %s (raw key not reissued)\n", resp.APIKey.KeyPrefix)
		}
		for _, artifact := range resp.Bundle.Artifacts {
			fmt.Fprintf(stdout, "\n--- %s (%s) ---\n%s\n", artifact.PathHint, artifact.Kind, artifact.Content)
		}
		printVerificationLinks(stdout, resp.Bundle.VerificationLinks)
		return nil
	}
	if cfg.noFiles {
		fmt.Fprintln(stdout, "No files written (--no-files).")
		printVerificationLinks(stdout, resp.Bundle.VerificationLinks)
		return nil
	}

	absOutputDir, err := filepath.Abs(cfg.outputDir)
	if err != nil {
		return fmt.Errorf("resolve output dir: %w", err)
	}
	written, err := onboarding.WriteArtifacts(resp.Bundle, absOutputDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Output dir: %s\n", absOutputDir)
	for _, item := range written {
		fmt.Fprintf(stdout, "Wrote: %s\n", item.Path)
	}
	if resp.APIKey != nil && strings.TrimSpace(resp.APIKey.RawKey) != "" {
		fmt.Fprintln(stdout, "Raw API key was returned once and written into generated env artifacts. Rotate or store it now if needed.")
	} else if resp.APIKey != nil && strings.TrimSpace(resp.APIKey.KeyPrefix) != "" {
		fmt.Fprintf(stdout, "Existing API key reference: %s (raw key not reissued)\n", resp.APIKey.KeyPrefix)
	}
	printVerificationLinks(stdout, resp.Bundle.VerificationLinks)
	return nil
}

func printVerificationLinks(stdout io.Writer, links []onboarding.VerificationLink) {
	if len(links) == 0 {
		return
	}
	fmt.Fprintln(stdout, "Verification links:")
	for _, link := range links {
		fmt.Fprintf(stdout, "- %s: %s\n", link.Label, link.Path)
	}
}

func humanMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "preview":
		return "Server preview"
	case "regenerated":
		return "Server regenerate"
	case "regenerated_defaults":
		return "Server regenerate with defaults"
	case "created":
		return "Server create"
	case "local":
		return "Local-only generation"
	default:
		if strings.TrimSpace(mode) == "" {
			return "Unknown"
		}
		return mode
	}
}

func resolvedAgentID(cfg *cliConfig) string {
	resolvedAgentID := strings.TrimSpace(cfg.agentID)
	if resolvedAgentID == "" {
		resolvedAgentID = "local-" + onboarding.SuggestedAgentID(cfg.agentName)
	}
	return resolvedAgentID
}

func resolvedTenantName(cfg *cliConfig) string {
	if strings.TrimSpace(cfg.tenantName) != "" {
		return strings.TrimSpace(cfg.tenantName)
	}
	if strings.TrimSpace(cfg.newTenantName) != "" {
		return strings.TrimSpace(cfg.newTenantName)
	}
	return strings.TrimSpace(cfg.tenantID)
}

func parseTools(value string) ([]onboarding.SelectedTool, error) {
	parts := strings.Split(strings.TrimSpace(value), ",")
	tools := make([]onboarding.SelectedTool, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		pieces := strings.SplitN(item, ":", 2)
		if len(pieces) != 2 || strings.TrimSpace(pieces[0]) == "" || strings.TrimSpace(pieces[1]) == "" {
			return nil, fmt.Errorf("tool %q must use tool:action form", item)
		}
		tools = append(tools, onboarding.SelectedTool{
			Tool:   strings.TrimSpace(pieces[0]),
			Action: strings.TrimSpace(pieces[1]),
		})
	}
	return tools, nil
}

type bridgeChatRequest struct {
	Model    string              `json:"model,omitempty"`
	Messages []bridgeChatMessage `json:"messages"`
}

type bridgeChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content,omitempty"`
}

type bridgeChatResponse struct {
	Choices []bridgeChatChoice `json:"choices"`
}

type bridgeChatChoice struct {
	Message bridgeChatMessage `json:"message"`
}

func initialChatConversation(systemPrompt string) []bridgeChatMessage {
	if strings.TrimSpace(systemPrompt) == "" {
		return []bridgeChatMessage{}
	}
	return []bridgeChatMessage{{
		Role:    "system",
		Content: strings.TrimSpace(systemPrompt),
	}}
}

func initialChatMessages(systemPrompt, prompt string) []bridgeChatMessage {
	messages := initialChatConversation(systemPrompt)
	return append(messages, bridgeChatMessage{
		Role:    "user",
		Content: strings.TrimSpace(prompt),
	})
}

func requestBridgeChat(baseURL, profile, model string, messages []bridgeChatMessage) (string, error) {
	body, err := json.Marshal(bridgeChatRequest{
		Model:    strings.TrimSpace(model),
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("encode bridge chat request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build bridge chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(profile) != "" {
		req.Header.Set("X-OpenClause-Profile", strings.TrimSpace(profile))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bridge chat request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read bridge chat response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bridge chat failed: %s", strings.TrimSpace(string(respBody)))
	}

	var out bridgeChatResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode bridge chat response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("bridge chat returned no choices")
	}
	text := strings.TrimSpace(chatContentText(out.Choices[0].Message.Content))
	if text == "" {
		return "", fmt.Errorf("bridge chat returned empty assistant content")
	}
	return text, nil
}

func chatContentText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if kind, _ := block["type"].(string); kind != "text" {
				continue
			}
			if text, _ := block["text"].(string); strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  openclause init-agent [--server-url URL [--auth-token TOKEN|--auth-profile NAME]] [--preview|--regenerate [--use-defaults]] [--local-only] --tenant-id TENANT|--new-tenant-name NAME --agent-name NAME --runtime python|typescript|langchain|openai_local --tools tool:action[,tool:action]")
	fmt.Fprintln(w, "  openclause auth login [--server-url URL] [--profile NAME] [--email EMAIL] [--password PASSWORD|--password-stdin]")
	fmt.Fprintln(w, "  openclause auth whoami [--server-url URL|--profile NAME]")
	fmt.Fprintln(w, "  openclause auth logout [--server-url URL|--profile NAME]")
	fmt.Fprintln(w, "  openclause bridge start --config ./openclause-bridge.yaml")
	fmt.Fprintln(w, "  openclause bridge chat --config ./openclause-bridge.yaml [--profile NAME] [--prompt \"hello\"]")
	fmt.Fprintln(w, "  openclause bridge doctor --config ./openclause-bridge.yaml [--profile NAME]")
	fmt.Fprintln(w, "  openclause bridge mcp --config ./openclause-bridge.yaml [--profile NAME]")
}

func printAuthUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  openclause auth login [--server-url URL] [--profile NAME] [--email EMAIL] [--password PASSWORD|--password-stdin]")
	fmt.Fprintln(w, "  openclause auth whoami [--server-url URL|--profile NAME]")
	fmt.Fprintln(w, "  openclause auth logout [--server-url URL|--profile NAME]")
}

func printBridgeUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  openclause bridge start --config ./openclause-bridge.yaml")
	fmt.Fprintln(w, "  openclause bridge chat --config ./openclause-bridge.yaml [--profile NAME] [--prompt \"hello\"]")
	fmt.Fprintln(w, "  openclause bridge doctor --config ./openclause-bridge.yaml [--profile NAME]")
	fmt.Fprintln(w, "  openclause bridge mcp --config ./openclause-bridge.yaml [--profile NAME]")
}
