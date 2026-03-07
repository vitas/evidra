package experiments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DefaultArtifactCasesDir    = "tests/benchmark/cases"
	DefaultExecutionScenarios  = "tests/experiments/execution-scenarios"
	DefaultPromptFile          = "prompts/experiments/runtime/system_instructions.txt"
	DefaultArtifactOutRoot     = "experiments/results"
	DefaultLLMBaselineOutRoot  = "experiments/results/llm"
	DefaultExecutionOutSuffix  = "-execution"
	ResultSchemaVersion        = "evidra.result.v1"
	ExecutionResultSchema      = "evidra.exec-result.v1"
	LLMBaselineSchemaVersion   = "evidra.llm-baseline.v1"
	defaultPromptVersion       = "v1"
	defaultPromptContractValue = "unknown"
)

var (
	ErrUnsupportedAgent = errors.New("unsupported agent")
)

type PromptInfo struct {
	File            string
	Version         string
	ContractVersion string
	SystemPrompt    string
}

func runStampNow() string {
	return time.Now().UTC().Format("20060102T150405Z")
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func safeModelID(v string) string {
	replacer := strings.NewReplacer("/", "-", ":", "-", " ", "-")
	s := replacer.Replace(v)
	var b strings.Builder
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.' || ch == '-' {
			b.WriteRune(ch)
		}
	}
	if b.Len() == 0 {
		return "unknown-model"
	}
	return b.String()
}

func normalizeTags(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func parseContractVersionAndBody(path string) (version string, body string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	lines := strings.Split(string(raw), "\n")
	contract := defaultPromptContractValue
	bodyLines := make([]string, 0, len(lines))
	skippedHeader := false

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if !skippedHeader {
			if trimmed == "" {
				continue
			}
			probe := trimmed
			if strings.HasPrefix(probe, "<!--") && strings.HasSuffix(probe, "-->") {
				probe = strings.TrimSuffix(strings.TrimPrefix(probe, "<!--"), "-->")
				probe = strings.TrimSpace(probe)
			}
			probe = strings.TrimPrefix(probe, "#")
			probe = strings.TrimSpace(probe)
			if strings.HasPrefix(strings.ToLower(probe), "contract:") {
				val := strings.TrimSpace(probe[len("contract:"):])
				if val != "" {
					contract = val
				}
				skippedHeader = true
				continue
			}
			skippedHeader = true
		}
		bodyLines = append(bodyLines, rawLine)
	}

	return contract, strings.TrimSpace(strings.Join(bodyLines, "\n")), nil
}

func resolvePromptInfo(promptFile, promptVersion string) (PromptInfo, error) {
	contractVersion, body, err := parseContractVersionAndBody(promptFile)
	if err != nil {
		return PromptInfo{}, fmt.Errorf("read prompt file: %w", err)
	}
	version := promptVersion
	if strings.TrimSpace(version) == "" {
		if contractVersion != "" && contractVersion != defaultPromptContractValue {
			version = contractVersion
		} else {
			version = defaultPromptVersion
		}
	}
	return PromptInfo{
		File:            promptFile,
		Version:         version,
		ContractVersion: contractVersion,
		SystemPrompt:    body,
	}, nil
}

func ensureDirClean(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("clean out dir requires non-empty path")
	}
	if path == "/" || path == "." || path == ".." {
		return fmt.Errorf("refuse to clean unsafe out dir: %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("out dir is not a directory: %s", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func listFilesByName(root, filename string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == filename {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func compileOptionalRegex(expr string) (*regexp.Regexp, error) {
	if strings.TrimSpace(expr) == "" {
		return nil, nil
	}
	return regexp.Compile(expr)
}

func writeJSONFile(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeTextFile(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}
