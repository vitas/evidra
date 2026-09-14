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

// orderedCommands is the entire CLI surface by design (§41): two commands plus
// version. Enforcement is not in this binary — it lives in the `evidra-mcp --proxy`
// endpoint, and the CLI only reads what that endpoint recorded. Commands outside this
// set are not part of the current product; docs/cli-reference.md is the canonical public
// command inventory.
var orderedCommands = []commandSpec{
	{name: "summarize", description: "Reconcile vNext MCP evidence: declared vs observed vs reported", run: cmdSummarize},
	{name: "verify", description: "Verify vNext evidence chains, signatures and coverage per recorder", run: cmdVerifyChain},
	{name: "version", description: "Print version information", run: cmdVersion},
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "evidra -- read side of the vNext MCP evidence recorder")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS:")
	for _, command := range orderedCommands {
		fmt.Fprintf(w, "  %-12s %s\n", command.name, command.description)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Evidence is written by: evidra-mcp --proxy --evidence-dir DIR -- <upstream command>")
	fmt.Fprintln(w, "Point these commands at a recorder root or at one recorder directory.")
}
