package promptfactory

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type renderData struct {
	ContractVersion string
	Contract        Contract
	Classification  Classification
	Output          OutputContracts
}

func RenderFiles(rootDir string, bundle Bundle) ([]RenderedFile, error) {
	specs := []struct {
		id        string
		template  string
		generated string
		active    string
	}{
		{
			id:        "mcp.initialize",
			template:  "templates/mcp/initialize.tmpl",
			generated: filepath.Join("prompts", "generated", bundle.Contract.Version, "mcpserver", "initialize", "instructions.txt"),
			active:    filepath.Join("prompts", "mcpserver", "initialize", "instructions.txt"),
		},
		{
			id:        "mcp.prescribe",
			template:  "templates/mcp/prescribe.tmpl",
			generated: filepath.Join("prompts", "generated", bundle.Contract.Version, "mcpserver", "tools", "prescribe_description.txt"),
			active:    filepath.Join("prompts", "mcpserver", "tools", "prescribe_description.txt"),
		},
		{
			id:        "mcp.report",
			template:  "templates/mcp/report.tmpl",
			generated: filepath.Join("prompts", "generated", bundle.Contract.Version, "mcpserver", "tools", "report_description.txt"),
			active:    filepath.Join("prompts", "mcpserver", "tools", "report_description.txt"),
		},
		{
			id:        "mcp.get_event",
			template:  "templates/mcp/get_event.tmpl",
			generated: filepath.Join("prompts", "generated", bundle.Contract.Version, "mcpserver", "tools", "get_event_description.txt"),
			active:    filepath.Join("prompts", "mcpserver", "tools", "get_event_description.txt"),
		},
		{
			id:        "mcp.agent_contract",
			template:  "templates/mcp/agent_contract.tmpl",
			generated: filepath.Join("prompts", "generated", bundle.Contract.Version, "mcpserver", "resources", "content", "agent_contract_v1.md"),
			active:    filepath.Join("prompts", "mcpserver", "resources", "content", "agent_contract_v1.md"),
		},
		{
			id:        "runtime.system",
			template:  "templates/runtime/system_instructions.tmpl",
			generated: filepath.Join("prompts", "generated", bundle.Contract.Version, "experiments", "runtime", "system_instructions.txt"),
			active:    filepath.Join("prompts", "experiments", "runtime", "system_instructions.txt"),
		},
		{
			id:        "runtime.agent_contract",
			template:  "templates/runtime/agent_contract.tmpl",
			generated: filepath.Join("prompts", "generated", bundle.Contract.Version, "experiments", "runtime", "agent_contract_v1.md"),
			active:    filepath.Join("prompts", "experiments", "runtime", "agent_contract_v1.md"),
		},
		{
			id:        "skill.skill",
			template:  "templates/skill/SKILL.tmpl",
			generated: filepath.Join("prompts", "generated", bundle.Contract.Version, "skill", "SKILL.md"),
			active:    filepath.Join("prompts", "skill", "SKILL.md"),
		},
	}

	data := renderData{
		ContractVersion: bundle.Contract.Version,
		Contract:        bundle.Contract,
		Classification:  bundle.Classification,
		Output:          bundle.Output,
	}

	templateBase := filepath.Join(rootDir, "prompts", "source", "contracts", bundle.Contract.Version)

	out := make([]RenderedFile, 0, len(specs))
	for _, spec := range specs {
		tplPath := filepath.Join(templateBase, spec.template)
		tplBytes, err := os.ReadFile(tplPath)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", tplPath, err)
		}
		tpl, err := template.New(spec.id).Option("missingkey=error").Parse(string(tplBytes))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", tplPath, err)
		}
		var buf bytes.Buffer
		if err := tpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("execute template %s: %w", tplPath, err)
		}

		content := normalizeRenderedContent(buf.String())
		out = append(out, RenderedFile{
			ID:          spec.id,
			TemplateRel: filepath.ToSlash(filepath.Join("prompts", "source", "contracts", bundle.Contract.Version, spec.template)),
			OutputRel:   filepath.ToSlash(spec.generated),
			ActiveRel:   filepath.ToSlash(spec.active),
			Content:     content,
		})
	}

	return out, nil
}

func normalizeRenderedContent(in string) string {
	in = strings.ReplaceAll(in, "\r\n", "\n")
	lines := strings.Split(in, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	out := strings.Join(lines, "\n")
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	return out + "\n"
}
