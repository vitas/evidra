package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"evidra/internal/demo/gitops"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: evidra-demo <seed-commit>")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "seed-commit":
		seedCommit(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func seedCommit(args []string) {
	fs := flag.NewFlagSet("seed-commit", flag.ExitOnError)
	repoURL := fs.String("repo-url", "", "repository url")
	username := fs.String("username", "", "git username")
	password := fs.String("password", "", "git password")
	commitCase := fs.String("case", "A", "commit case: A|B|C")
	_ = fs.Parse(args)

	if strings.TrimSpace(*repoURL) == "" {
		fmt.Fprintln(os.Stderr, "--repo-url is required")
		os.Exit(2)
	}

	cc := gitops.CommitCase(strings.ToUpper(strings.TrimSpace(*commitCase)))
	if cc != gitops.CaseA && cc != gitops.CaseB && cc != gitops.CaseC {
		fmt.Fprintln(os.Stderr, "--case must be one of: A, B, C")
		os.Exit(2)
	}

	client := gitops.Client{RepoURL: *repoURL, Username: *username, Password: *password}
	hash, err := client.PushCase(context.Background(), cc)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if strings.TrimSpace(hash) == "" {
		fmt.Printf("No new commit created for case %s (content unchanged).\n", cc)
		return
	}
	fmt.Printf("Pushed case %s commit: %s\n", cc, hash)
}
