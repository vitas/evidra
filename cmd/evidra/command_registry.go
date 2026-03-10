package main

import (
	"fmt"
	"io"
)

type commandHandler func(args []string, stdout, stderr io.Writer) int

type commandSpec struct {
	name        string
	description string
	run         commandHandler
}

func registeredCommands() map[string]commandSpec {
	commands := make(map[string]commandSpec, len(orderedCommands))
	for _, command := range orderedCommands {
		commands[command.name] = command
	}
	return commands
}

func lookupCommand(name string) (commandSpec, bool) {
	command, ok := registeredCommands()[name]
	return command, ok
}

var orderedCommands = []commandSpec{
	{name: "scorecard", description: "Generate reliability scorecard for an actor", run: cmdScorecard},
	{name: "explain", description: "Explain signals contributing to a score", run: cmdExplain},
	{name: "compare", description: "Compare reliability scores between actors", run: cmdCompare},
	{name: "run", description: "Execute command live and record lifecycle outcome", run: cmdRun},
	{name: "prescribe", description: "Analyze artifact before execution", run: cmdPrescribe},
	{name: "report", description: "Record execution outcome or declined decision", run: cmdReport},
	{name: "record", description: "Ingest completed automation operation from structured input", run: cmdRecord},
	{name: "validate", description: "Validate evidence chain integrity and signatures", run: cmdValidate},
	{name: "ingest-findings", description: "Ingest SARIF scanner findings as evidence entries", run: cmdIngestFindings},
	{name: "prompts", description: "Prompt contract generation and verification", run: cmdPrompts},
	{name: "detectors", description: "Detector registry command group", run: cmdDetectors},
	{name: "keygen", description: "Generate Ed25519 signing keypair", run: cmdKeygen},
	{name: "version", description: "Print version information", run: cmdVersion},
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra-benchmark — reliability benchmark for infrastructure automation")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS:")
	for _, command := range orderedCommands {
		fmt.Fprintf(w, "  %-18s %s\n", command.name, command.description)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'evidra <command> --help' for command-specific flags.")
}
