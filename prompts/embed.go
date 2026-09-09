package prompts

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	MCPInitializeInstructionsPath    = "mcpserver/initialize/instructions.txt"
	MCPRunCommandDescriptionPath     = "mcpserver/tools/run_command_description.txt"
	MCPPrescribeFullDescriptionPath  = "mcpserver/tools/prescribe_full_description.txt"
	MCPPrescribeSmartDescriptionPath = "mcpserver/tools/prescribe_smart_description.txt"
	MCPReportDescriptionPath         = "mcpserver/tools/report_description.txt"
	MCPGetEventDescriptionPath       = "mcpserver/tools/get_event_description.txt"
	MCPAgentContractPath             = "mcpserver/resources/content/agent_contract_v1.md"
	MCPPromptPrescribeSmartPath      = "mcp/prompt_prescribe_smart.md"
	MCPPromptPrescribeFullPath       = "mcp/prompt_prescribe_full.md"
	MCPPromptDiagnosisPath           = "mcp/prompt_diagnosis.md"
	SkillPath                        = "skill/SKILL.md"
	SkillSmartPath                   = "skill/SKILL_SMART.md"
	SkillFullPath                    = "skill/SKILL_FULL.md"

	DefaultContractVersion      = "v1.4.0"
	DefaultContractSkillVersion = "1.4.0"
)

var (
	contractVersionPattern = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,2}$`)

	//go:embed mcpserver/initialize/instructions.txt mcpserver/tools/run_command_description.txt mcpserver/tools/prescribe_full_description.txt mcpserver/tools/prescribe_smart_description.txt mcpserver/tools/report_description.txt mcpserver/tools/get_event_description.txt mcpserver/resources/content/agent_contract_v1.md skill/SKILL.md skill/SKILL_SMART.md skill/SKILL_FULL.md mcp/prompt_prescribe_smart.md mcp/prompt_prescribe_full.md mcp/prompt_diagnosis.md manifests/*.json
	files embed.FS
)

// Metadata captures contract and prompt provenance for a prompt file.
type Metadata struct {
	Path            string
	ContractVersion string
	SkillVersion    string
	PromptVersion   string
}

type manifestFile struct {
	Files map[string]string `json:"files"`
}

func Read(path string) (string, error) {
	b, err := files.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func ReadSkill() (string, error) {
	return Read(SkillPath)
}

func ReadSkillFull() (string, error) {
	return Read(SkillFullPath)
}

func ReadMCPInitializeInstructions() (instructions string, contractVersion string, skillVersion string, err error) {
	raw, err := Read(MCPInitializeInstructionsPath)
	if err != nil {
		return "", DefaultContractVersion, DefaultContractSkillVersion, err
	}

	contractVersion, ok := parseContractVersionHeader(raw)
	if !ok {
		contractVersion = DefaultContractVersion
	}
	skillVersion = skillVersionFromContractVersion(contractVersion)

	body := stripContractHeader(raw)
	if body == "" {
		body = "Evidra — Flight recorder for AI infrastructure agents."
	}
	instructions = strings.TrimSpace(body) + "\n\nContract version: " + contractVersion + " (skill_version=" + skillVersion + ")"
	return instructions, contractVersion, skillVersion, nil
}

func ParseSkillVersionFromContractVersion(contractVersion string) string {
	return skillVersionFromContractVersion(contractVersion)
}

func StripContractHeader(text string) string {
	return stripContractHeader(text)
}

func ParseContractVersionHeader(text string) (string, bool) {
	return parseContractVersionHeader(text)
}

func ResolvePromptMetadata(path string) (Metadata, error) {
	normalized := normalizePromptPath(path)
	if normalized == "" {
		return Metadata{}, fmt.Errorf("prompt path is required")
	}
	if strings.HasPrefix(normalized, "prompts/experiments/runtime/") || strings.Contains(normalized, "/experiments/runtime/") {
		return Metadata{}, fmt.Errorf("prompt path %q is no longer supported", path)
	}

	content, err := readPromptContent(path, normalized)
	if err != nil {
		return Metadata{}, err
	}

	contractVersion, ok := parseContractVersionHeader(content)
	if !ok {
		contractVersion = DefaultContractVersion
	}

	promptVersion, err := manifestPromptVersion(contractVersion, normalized)
	if err != nil {
		return Metadata{}, err
	}

	return Metadata{
		Path:            normalized,
		ContractVersion: contractVersion,
		SkillVersion:    skillVersionFromContractVersion(contractVersion),
		PromptVersion:   promptVersion,
	}, nil
}

func parseContractVersionHeader(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") && strings.HasSuffix(trimmed, "-->") {
			trimmed = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "<!--"), "-->"))
		}
		if strings.HasPrefix(trimmed, "#") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		}
		if !strings.HasPrefix(strings.ToLower(trimmed), "contract:") {
			return "", false
		}
		value := strings.TrimSpace(trimmed[len("contract:"):])
		if value == "" {
			return "", false
		}
		if !contractVersionPattern.MatchString(value) {
			return "", false
		}
		if !strings.HasPrefix(value, "v") {
			value = "v" + value
		}
		return value, true
	}
	return "", false
}

func stripContractHeader(text string) string {
	lines := strings.Split(text, "\n")
	idx := 0
	for idx < len(lines) {
		line := strings.TrimSpace(lines[idx])
		if line == "" {
			idx++
			continue
		}
		if _, ok := parseContractVersionHeader(line); ok {
			idx++
		}
		break
	}
	return strings.TrimSpace(strings.Join(lines[idx:], "\n"))
}

func skillVersionFromContractVersion(contractVersion string) string {
	v := strings.TrimSpace(strings.TrimPrefix(contractVersion, "v"))
	if v == "" {
		return DefaultContractSkillVersion
	}
	parts := strings.Split(v, ".")
	if len(parts) == 2 {
		parts = append(parts, "0")
	}
	if len(parts) != 3 {
		return DefaultContractSkillVersion
	}
	for _, p := range parts {
		if p == "" || strings.TrimLeft(p, "0123456789") != "" {
			return DefaultContractSkillVersion
		}
	}
	return strings.Join(parts, ".")
}

func normalizePromptPath(path string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	if normalized == "" {
		return ""
	}
	if idx := strings.Index(normalized, "/prompts/"); idx >= 0 {
		return normalized[idx+1:]
	}
	if strings.HasPrefix(normalized, "prompts/") {
		return normalized
	}
	switch {
	case strings.HasPrefix(normalized, "experiments/"),
		strings.HasPrefix(normalized, "generated/"),
		strings.HasPrefix(normalized, "mcp/"),
		strings.HasPrefix(normalized, "mcpserver/"),
		strings.HasPrefix(normalized, "skill/"):
		return "prompts/" + normalized
	default:
		return normalized
	}
}

func readPromptContent(path, normalized string) (string, error) {
	if trimmed := strings.TrimSpace(path); trimmed != "" {
		if data, err := os.ReadFile(trimmed); err == nil {
			return string(data), nil
		}
	}
	embedPath := strings.TrimPrefix(normalized, "prompts/")
	data, err := files.ReadFile(embedPath)
	if err != nil {
		return "", fmt.Errorf("read prompt %q: %w", normalized, err)
	}
	return string(data), nil
}

func manifestPromptVersion(contractVersion, normalizedPath string) (string, error) {
	raw, err := files.ReadFile("manifests/" + contractVersion + ".json")
	if err != nil {
		return "", fmt.Errorf("read manifest for %s: %w", contractVersion, err)
	}

	var manifest manifestFile
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("parse manifest for %s: %w", contractVersion, err)
	}

	promptVersion := manifest.Files[normalizedPath]
	if promptVersion == "" {
		return "", fmt.Errorf("prompt %q not found in manifest %s", normalizedPath, contractVersion)
	}
	return promptVersion, nil
}
