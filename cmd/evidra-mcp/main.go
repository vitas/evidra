package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

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
	evidenceFlag := fs.String("evidence-dir", "", "Root for vNext recorder directories (one per process); omit to enforce without recording")
	proxyModes := registerProxyFlags(fs)
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

	logger := log.New(stderr, "", log.LstdFlags)
	if code, handled := proxyModes.dispatch(context.Background(), stderr, *evidenceFlag, logger, fs.Args()); handled {
		return code
	}

	// Reaching this point means no wrapping mode was requested. The direct MCP
	// server mode is gone with §43: what Evidra sells is an endpoint in front of a
	// real upstream whose tools the agent prescribes against, so standing up alone
	// would serve a tool surface that no longer exists. Failing with usage beats
	// serving something an agent would then have to discover is hollow.
	fmt.Fprintln(stderr, "evidra-mcp: nothing to wrap; run with --proxy -- <upstream command>")
	printHelp(stderr)
	return 2
}

// proxyFlags holds the wrapping-mode flags, registered in one helper so that
// run() keeps a single dispatch statement.
type proxyFlags struct {
	wrap       *bool
	serverName *string
	advertise  *bool
	enforce    *string
	maxMessage *string
	// actorID labels who is accountable in every evidence event (§13). The default is
	// read at registration time from EVIDRA_ACTOR_ID on purpose: the envelope always
	// carries an actor, so with neither flag nor set the endpoint records its fallback
	// and the reader can see that in the chain rather than guessing.
	actorID *string
}

// Pointers are held deliberately: reading the values at registration time would
// freeze the defaults and the dispatch would never see what the user passed.
func registerProxyFlags(fs *flag.FlagSet) *proxyFlags {
	return &proxyFlags{
		wrap:       fs.Bool("proxy", false, "Merged endpoint: wrap one upstream MCP server and expose evidra_prescribe / evidra_report over its tools"),
		serverName: fs.String("server-name", "", "Label for the wrapped upstream in evidence and serverInfo"),
		advertise:  fs.Bool("advertise-passthrough", false, "Advertise upstream prompts/resources/completions that are relayed but outside the supported profile"),
		enforce:    fs.String("enforce", "all", "Protocol enforcement for the merged endpoint: all (default) or off (observe-only)"),
		maxMessage: fs.String("max-message", "64MiB", "Largest single JSON-RPC message accepted in either direction"),
		actorID:    fs.String("actor-id", os.Getenv("EVIDRA_ACTOR_ID"), "Actor identity recorded as accountable for this session's executions"),
	}
}

// dispatch routes to the requested wrapping mode, reporting whether one was
// asked for at all.
func (p *proxyFlags) dispatch(ctx context.Context, stderr io.Writer, evidenceRaw string, logger *log.Logger, args []string) (int, bool) {
	switch {
	case *p.wrap:
		return runEndpointMode(ctx, stderr, logger, args, endpointFlags{
			serverName:     *p.serverName,
			advertiseExtra: *p.advertise,
			maxMessage:     *p.maxMessage,
			enforce:        *p.enforce,
			actorID:        *p.actorID,
			evidenceDir:    evidenceRaw,
		}), true
	default:
		return 0, false
	}
}

// endpointFlags carries the merged-endpoint options parsed in run().
type endpointFlags struct {
	serverName     string
	advertiseExtra bool
	maxMessage     string
	enforce        string
	evidenceDir    string
	actorID        string
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
	if flags.evidenceDir == "" {
		// Recording is opt-in per process on purpose: the plan's default root is
		// ~/.evidra/evidence, and silently appending evidence to a home directory
		// from every test and dogfood run is a side effect nobody asked for. The
		// notice keeps the asymmetry visible instead of quiet.
		logger.Printf("evidra: --evidence-dir not set, so executions are enforced but not recorded")
	} else {
		logger.Printf("evidra: evidence root %s (one recorder directory per process)", flags.evidenceDir)
	}
	if err := proxy.RunEndpoint(ctx, os.Stdin, os.Stdout, proxy.RunEndpointOptions{
		UpstreamArgs:         remaining,
		Logger:               logger,
		MaxMessage:           maxMessage,
		AdvertisePassthrough: flags.advertiseExtra,
		ServerName:           flags.serverName,
		EnforceMode:          flags.enforce,
		EvidenceDir:          flags.evidenceDir,
		ActorID:              flags.actorID,
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

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "evidra-mcp — MCP endpoint that records execution evidence for one upstream server.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE:")
	fmt.Fprintln(w, "  evidra-mcp --proxy [flags] -- <upstream-command> [args...]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLE:")
	fmt.Fprintln(w, "  evidra-mcp --proxy --evidence-dir /tmp/evidence -- ./bin/evidra-fixture")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "The endpoint merges two local tools into the upstream tool list: evidra_prescribe")
	fmt.Fprintln(w, "(open an operation) and evidra_report (close it). Upstream tool calls are refused")
	fmt.Fprintln(w, "unless an operation is open, and every accepted call is recorded as a paired")
	fmt.Fprintln(w, "execution. Evidence is read back with: evidra summarize / evidra verify.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS:")
	fmt.Fprintln(w, "  --proxy                 Wrap the upstream command as the merged endpoint")
	fmt.Fprintln(w, "  --enforce <all|off>    all (default): refuse calls outside an operation;")
	fmt.Fprintln(w, "                          off: observe only, no refusals")
	fmt.Fprintln(w, "  --evidence-dir <dir>    Recorder root; one dated directory per process. Omit it")
	fmt.Fprintln(w, "                          to enforce without writing evidence")
	fmt.Fprintln(w, "  --server-name <label>    serverInfo.name and evidence label for the wrapper")
	fmt.Fprintln(w, "  --advertise-passthrough  Advertise relayed prompts/resources/completions that are")
	fmt.Fprintln(w, "                          outside the supported profile (default: not advertised)")
	fmt.Fprintln(w, "  --max-message <size>     Largest single JSON-RPC message accepted (default 64MiB)")
	fmt.Fprintln(w, "  --actor-id <id>           Actor recorded as accountable (default: $EVIDRA_ACTOR_ID,")
	fmt.Fprintln(w, "                            else the recorder's fallback identity)")
	fmt.Fprintln(w, "  --version                Print version and exit")
	fmt.Fprintln(w, "  --help                   Show this help")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "There is no default evidence location for the endpoint: recording is chosen per")
	fmt.Fprintln(w, "process with --evidence-dir, so a test or dogfood run cannot silently append to")
	fmt.Fprintln(w, "a home directory nothing pointed at. The read side (evidra summarize, evidra")
	fmt.Fprintln(w, "verify) does honour EVIDRA_EVIDENCE_DIR.")
}
