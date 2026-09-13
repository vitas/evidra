package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"samebits.com/evidra/internal/config"
	ievsigner "samebits.com/evidra/internal/evidence"
	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/mcpserver"
	"samebits.com/evidra/pkg/mode"
	"samebits.com/evidra/pkg/proxy"
	"samebits.com/evidra/pkg/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("evidra-mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	showVersion := fs.Bool("version", false, "Print version information and exit")
	evidenceFlag := fs.String("evidence-dir", "", "Path to store evidence records")
	environmentFlag := fs.String("environment", "", "Environment label (production, staging, development)")
	retryFlag := fs.Bool("retry-tracker", false, "Enable retry loop tracking")
	signingModeFlag := fs.String("signing-mode", "", "Signing mode: strict (default) or optional")
	urlFlag := fs.String("url", os.Getenv("EVIDRA_URL"), "Evidra API URL")
	apiKeyFlag := fs.String("api-key", os.Getenv("EVIDRA_API_KEY"), "Evidra API key")
	offlineFlag := fs.Bool("offline", false, "Force offline mode")
	fallbackOfflineFlag := fs.Bool("fallback-offline", false, "Fall back to offline on API failure")
	proxyFlag := fs.Bool("proxy", false, "Merged endpoint: wrap one upstream MCP server and expose evidra_prescribe / evidra_report over its tools")
	legacyProxyFlag := fs.Bool("legacy-proxy", false, "Pre-vNext relay: auto-record mutations with the legacy evidence writer (deprecated, slated for removal)")
	serverNameFlag := fs.String("server-name", "", "Label for the wrapped upstream in evidence and serverInfo")
	advertisePassthroughFlag := fs.Bool("advertise-passthrough", false, "Advertise upstream prompts/resources/completions that are relayed but outside the supported profile")
	maxMessageFlag := fs.String("max-message", "64MiB", "Largest single JSON-RPC message accepted in either direction")
	fullPrescribeFlag := fs.Bool("full-prescribe", false, "Expose prescribe_full tool (experimental, for advanced models only)")
	transportFlag := fs.String("transport", "stdio", "Transport mode: stdio (default) or streamable-http")
	portFlag := fs.String("port", "3001", "HTTP port when using streamable-http transport")
	actorIDFlag := fs.String("actor-id", os.Getenv("EVIDRA_ACTOR_ID"), "Actor identity for evidence entries (default: infrastructure-agent)")
	helpFlag := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version.BuildString("evidra-mcp"))
		return 0
	}
	if *helpFlag {
		printHelp(stderr)
		return 0
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)
	environment := resolveEnvironment(*environmentFlag)
	logger := log.New(stderr, "", log.LstdFlags)

	if *proxyFlag {
		return runEndpointMode(context.Background(), stderr, logger, fs.Args(), endpointFlags{
			serverName:     *serverNameFlag,
			advertiseExtra: *advertisePassthroughFlag,
			maxMessage:     *maxMessageFlag,
		})
	}
	if *legacyProxyFlag {
		return runProxyMode(context.Background(), stderr, evidencePath, logger, fs.Args())
	}

	writeMode, writeModeErr := config.ResolveEvidenceWriteMode("")
	if writeModeErr != nil {
		fmt.Fprintf(stderr, "resolve evidence write mode: %v\n", writeModeErr)
		return 1
	}

	signer, signerErr := resolveSigner(*signingModeFlag)
	if signerErr != nil {
		fmt.Fprintf(stderr, "resolve signer: %v\n", signerErr)
		return 1
	}

	// Resolve online/offline mode.
	fallbackPolicy := ""
	if *fallbackOfflineFlag {
		fallbackPolicy = "offline"
	}
	if v := os.Getenv("EVIDRA_FALLBACK"); v != "" && fallbackPolicy == "" {
		fallbackPolicy = v
	}
	resolved, modeErr := mode.Resolve(mode.Config{
		URL:            *urlFlag,
		APIKey:         *apiKeyFlag,
		FallbackPolicy: fallbackPolicy,
		ForceOffline:   *offlineFlag,
		Timeout:        30 * time.Second,
	})
	if modeErr != nil {
		fmt.Fprintf(stderr, "resolve mode: %v\n", modeErr)
		return 1
	}

	var forwardFn mcpserver.ForwardFunc
	if resolved.IsOnline {
		apiClient := resolved.Client
		forwardFn = func(ctx context.Context, entry json.RawMessage) {
			if _, fwdErr := apiClient.Forward(ctx, entry); fwdErr != nil {
				log.New(stderr, "", log.LstdFlags).Printf("warning: forward evidence: %v", fwdErr)
			}
		}
	}

	server, cleanup, err := mcpserver.NewServerWithCleanup(mcpserver.Options{
		Name:              "evidra-mcp",
		Version:           version.Version,
		EvidencePath:      evidencePath,
		Environment:       environment,
		ActorID:           *actorIDFlag,
		RetryTracker:      *retryFlag || envBool("EVIDRA_RETRY_TRACKER", false),
		BestEffortWrites:  writeMode == config.EvidenceWriteModeBestEffort,
		HidePrescribeFull: !*fullPrescribeFlag,
		Signer:            signer,
		Forward:           forwardFn,
	})
	if err != nil {
		fmt.Fprintf(stderr, "initialize server: %v\n", err)
		return 1
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			log.New(stderr, "", log.LstdFlags).Printf("warning: cleanup mcp service: %v", cleanupErr)
		}
	}()

	logger.Printf("evidra-mcp running (evidence: %s, env: %s, transport: %s)", evidencePath, environment, *transportFlag)

	switch *transportFlag {
	case "stdio":
		if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			fmt.Fprintf(stderr, "run mcp server: %v\n", err)
			return 1
		}
	case "streamable-http":
		handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
			return server
		}, nil)
		httpMux := http.NewServeMux()
		httpMux.Handle("/mcp", handler)
		httpMux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok\n"))
		})
		addr := ":" + *portFlag
		logger.Printf("evidra-mcp HTTP listening on %s", addr)
		if err := http.ListenAndServe(addr, httpMux); err != nil {
			fmt.Fprintf(stderr, "http listen: %v\n", err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "unsupported transport: %s (use stdio or streamable-http)\n", *transportFlag)
		return 1
	}
	return 0
}

// endpointFlags carries the merged-endpoint options parsed in run().
type endpointFlags struct {
	serverName     string
	advertiseExtra bool
	maxMessage     string
}

// runEndpointMode serves the vNext merged endpoint (§59 step 2): one upstream
// child process, Evidra's local protocol tools merged into its tool list, and a
// client-facing capability set limited to what the supported profile covers.
func runEndpointMode(ctx context.Context, stderr io.Writer, logger *log.Logger, args []string, flags endpointFlags) int {
	remaining, err := normalizeProxyArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	maxMessage, err := parseByteSize(flags.maxMessage)
	if err != nil {
		fmt.Fprintf(stderr, "invalid --max-message: %v\n", err)
		return 1
	}
	logger.Printf("evidra-mcp merged endpoint (upstream: %s, server-name: %s)", remaining[0], flags.serverName)
	if err := proxy.RunEndpoint(ctx, os.Stdin, os.Stdout, proxy.RunEndpointOptions{
		UpstreamArgs:         remaining,
		Logger:               logger,
		MaxMessage:           maxMessage,
		AdvertisePassthrough: flags.advertiseExtra,
		ServerName:           flags.serverName,
	}); err != nil {
		fmt.Fprintf(stderr, "endpoint: %v\n", err)
		return 1
	}
	return 0
}

// parseByteSize accepts plain bytes plus KiB/MiB/GiB and the kb/mb/gb spellings.
func parseByteSize(v string) (int, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	digits, unit := s[:i], strings.ToLower(strings.ReplaceAll(s[i:], " ", ""))
	if digits == "" {
		return 0, fmt.Errorf("invalid size %q", v)
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", v)
	}
	mult := 1
	switch unit {
	case "":
	case "k", "kb":
		mult = 1 << 10
	case "kib":
		mult = 1 << 10
	case "m", "mb":
		mult = 1 << 20
	case "mib":
		mult = 1 << 20
	case "g", "gb":
		mult = 1 << 30
	case "gib":
		mult = 1 << 30
	default:
		return 0, fmt.Errorf("unknown unit %q", s[i:])
	}
	return n * mult, nil
}

func normalizeProxyArgs(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("proxy mode requires upstream command after --")
	}
	if args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("proxy mode requires upstream command after --")
	}
	return args, nil
}

func runProxyMode(ctx context.Context, stderr io.Writer, evidencePath string, logger *log.Logger, args []string) int {
	remaining, err := normalizeProxyArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	evidenceWriter, err := proxy.NewEvidenceWriter(evidencePath)
	if err != nil {
		fmt.Fprintf(stderr, "proxy evidence: %v\n", err)
		return 1
	}
	defer func() {
		if closeErr := evidenceWriter.Close(); closeErr != nil {
			logger.Printf("warning: close proxy evidence writer: %v", closeErr)
		}
	}()

	p := &proxy.Proxy{
		UpstreamCmd:  remaining[0],
		UpstreamArgs: remaining[1:],
		Evidence:     evidenceWriter,
		Verbose:      envBool("EVIDRA_PROXY_VERBOSE", false),
	}

	logger.Printf("evidra-mcp proxy mode (upstream: %s, evidence: %s)", remaining[0], evidenceWriter.Dir())

	if err := p.Run(ctx); err != nil {
		fmt.Fprintf(stderr, "proxy: %v\n", err)
		return 1
	}
	return 0
}

func resolveEvidencePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := strings.TrimSpace(os.Getenv("EVIDRA_EVIDENCE_DIR")); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".evidra", "evidence")
	}
	return filepath.Join(home, ".evidra", "evidence")
}

func resolveEnvironment(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := strings.TrimSpace(os.Getenv("EVIDRA_ENVIRONMENT")); v != "" {
		return v
	}
	return ""
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes":
		return true
	case "0", "false", "no":
		return false
	}
	return fallback
}

// resolveSigner creates a Signer from environment variables.
// Returns an error when mode is strict and no key is configured.
func resolveSigner(modeRaw string) (evidence.Signer, error) {
	mode, err := config.ResolveSigningMode(modeRaw)
	if err != nil {
		return nil, err
	}

	keyBase64 := strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY"))
	keyPath := strings.TrimSpace(os.Getenv("EVIDRA_SIGNING_KEY_PATH"))

	noKey := keyBase64 == "" && keyPath == ""
	if noKey && mode == config.SigningModeStrict {
		return nil, fmt.Errorf("signing key required in strict mode: set EVIDRA_SIGNING_KEY or EVIDRA_SIGNING_KEY_PATH (or --signing-mode optional)")
	}

	s, err := ievsigner.NewSigner(ievsigner.SignerConfig{
		KeyBase64: keyBase64,
		KeyPath:   keyPath,
		DevMode:   noKey && mode == config.SigningModeOptional,
	})
	if err != nil {
		return nil, fmt.Errorf("resolveSigner: %w", err)
	}
	return s, nil
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "evidra-mcp — MCP integration point for infrastructure automation reliability (including AI agents).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "MODES:")
	fmt.Fprintln(w, "  Direct (default)  run_command, collect_diagnostics, write_file, describe_tool, prescribe_smart, report, get_event")
	fmt.Fprintln(w, "  Direct (+ flag)   Add prescribe_full with --full-prescribe for artifact-aware explicit intent capture")
	fmt.Fprintln(w, "  Proxy (--proxy)   Wraps upstream MCP server, auto-records mutations")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  evidra-mcp [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "PROXY USAGE:")
	fmt.Fprintln(w, "  evidra-mcp --proxy [flags] -- <upstream-command> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "PROXY EXAMPLE:")
	fmt.Fprintln(w, "  evidra-mcp --proxy -- kubectl-mcp-server")
	fmt.Fprintln(w, "  evidra-mcp --proxy --evidence-dir /tmp/evidence -- npx -y @example/kubectl-mcp")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS:")
	fmt.Fprintln(w, "  --evidence-dir <dir>    Where to store evidence chain (default: ~/.evidra/evidence)")
	fmt.Fprintln(w, "  --environment <label>   Environment label (production, staging, development)")
	fmt.Fprintln(w, "  --retry-tracker         Enable retry loop tracking")
	fmt.Fprintln(w, "  --signing-mode <mode>   Signing mode: strict (default) or optional")
	fmt.Fprintln(w, "  --full-prescribe        Expose prescribe_full alongside the default direct tool surface")
	fmt.Fprintln(w, "  --proxy                 Enable proxy mode (wrap upstream MCP server)")
	fmt.Fprintln(w, "  --version               Print version and exit")
	fmt.Fprintln(w, "  --help                  Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "ENVIRONMENT:")
	fmt.Fprintln(w, "  EVIDRA_EVIDENCE_DIR     Override evidence directory")
	fmt.Fprintln(w, "  EVIDRA_ENVIRONMENT      Default environment label")
	fmt.Fprintln(w, "  EVIDRA_RETRY_TRACKER    Enable retry tracking (true/false)")
	fmt.Fprintln(w, "  EVIDRA_EVIDENCE_WRITE_MODE  strict (default) or best_effort")
	fmt.Fprintln(w, "  EVIDRA_SIGNING_MODE     strict (default) or optional")
	fmt.Fprintln(w, "  EVIDRA_PROXY_VERBOSE    Enable verbose proxy logging (true/false)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "WORKFLOW:")
	fmt.Fprintln(w, "  Use run_command for the normal workflow. Evidra adds auto-evidence for mutations.")
	fmt.Fprintln(w, "  Use describe_tool only when you want explicit prescribe_smart/report control.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "TOOLS (default direct mode):")
	fmt.Fprintln(w, "  run_command      Execute kubectl/helm/terraform/aws with smart output and auto-evidence for mutations")
	fmt.Fprintln(w, "  collect_diagnostics  Run one bundled Kubernetes diagnosis pass for a workload")
	fmt.Fprintln(w, "  write_file       Write files under cwd or temp directories; blocks system dirs and ~/.ssh")
	fmt.Fprintln(w, "  describe_tool    Show the full schema for deferred protocol tools when you need explicit control")
	fmt.Fprintln(w, "  prescribe_smart  Deferred by default; use describe_tool first for the full explicit-control schema")
	fmt.Fprintln(w, "  report           Deferred by default; use describe_tool first for the full explicit-control schema")
	fmt.Fprintln(w, "  get_event        Look up evidence record by event_id")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "OPTIONAL TOOL (--full-prescribe):")
	fmt.Fprintln(w, "  prescribe_full   Analyze artifact BEFORE execution (returns risk + prescription_id)")
}
