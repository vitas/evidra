package prompts

import (
	"embed"
	"regexp"
	"strings"
)

const (
	MCPInitializeInstructionsPath = "mcpserver/initialize/instructions.txt"
	MCPPrescribeDescriptionPath   = "mcpserver/tools/prescribe_description.txt"
	MCPReportDescriptionPath      = "mcpserver/tools/report_description.txt"
	MCPGetEventDescriptionPath    = "mcpserver/tools/get_event_description.txt"
	MCPAgentContractPath          = "mcpserver/resources/content/agent_contract_v1.md"

	defaultContractVersion      = "v1.0.1"
	defaultContractSkillVersion = "1.0.1"
)

var (
	contractVersionPattern = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+){1,2}$`)

	//go:embed mcpserver/initialize/instructions.txt mcpserver/tools/prescribe_description.txt mcpserver/tools/report_description.txt mcpserver/tools/get_event_description.txt mcpserver/resources/content/agent_contract_v1.md
	files embed.FS
)

func Read(path string) (string, error) {
	b, err := files.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func ReadMCPInitializeInstructions() (instructions string, contractVersion string, skillVersion string, err error) {
	raw, err := Read(MCPInitializeInstructionsPath)
	if err != nil {
		return "", defaultContractVersion, defaultContractSkillVersion, err
	}

	contractVersion, ok := parseContractVersionHeader(raw)
	if !ok {
		contractVersion = defaultContractVersion
	}
	skillVersion = skillVersionFromContractVersion(contractVersion)

	body := stripContractHeader(raw)
	if body == "" {
		body = "Evidra — behavioral reliability for infrastructure automation."
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
		return defaultContractSkillVersion
	}
	parts := strings.Split(v, ".")
	if len(parts) == 2 {
		parts = append(parts, "0")
	}
	if len(parts) != 3 {
		return defaultContractSkillVersion
	}
	for _, p := range parts {
		if p == "" || strings.TrimLeft(p, "0123456789") != "" {
			return defaultContractSkillVersion
		}
	}
	return strings.Join(parts, ".")
}
