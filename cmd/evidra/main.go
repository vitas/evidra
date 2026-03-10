package main

import (
	"fmt"
	"io"
	"os"

	"samebits.com/evidra-benchmark/pkg/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "--version", "-version":
		return cmdVersion(nil, stdout, stderr)
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	}

	command, ok := lookupCommand(args[0])
	if !ok {
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
	return command.run(args[1:], stdout, stderr)
}

func cmdVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, "evidra-benchmark %s (commit: %s, built: %s)\n",
		version.Version, version.Commit, version.Date)
	return 0
}
