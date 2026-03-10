package main

import (
	"flag"
	"fmt"
	"io"

	ievsigner "samebits.com/evidra-benchmark/internal/evidence"
	"samebits.com/evidra-benchmark/pkg/evidence"
)

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	evidenceFlag := fs.String("evidence-dir", "", "Evidence directory")
	pubKeyFlag := fs.String("public-key", "", "PEM file with Ed25519 public key (enables signature verification)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	evidencePath := resolveEvidencePath(*evidenceFlag)
	if err := evidence.ValidateChainAtPath(evidencePath); err != nil {
		fmt.Fprintf(stderr, "chain validation failed: %v\n", err)
		return 1
	}

	if *pubKeyFlag != "" {
		pubKey, err := ievsigner.LoadPublicKeyPEM(*pubKeyFlag)
		if err != nil {
			fmt.Fprintf(stderr, "load public key: %v\n", err)
			return 1
		}
		if err := evidence.ValidateChainWithSignatures(evidencePath, pubKey); err != nil {
			fmt.Fprintf(stderr, "signature validation failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "chain valid: hashes and signatures verified")
	} else {
		fmt.Fprintln(stdout, "chain valid: hashes verified (no public key provided, signatures not checked)")
	}
	return 0
}
