package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// epSupportedProtocolVersions are the versions the endpoint will name in an
// initialize result. Negotiation is explicit rather than assumed (§28).
var epSupportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// epLatestProtocolVersion is what the endpoint asks the upstream for.
const epLatestProtocolVersion = "2025-06-18"

// epReservedTools are the namespaced local protocol tools. An upstream that
// advertises either of them makes the merge ambiguous, so the endpoint refuses
// to start instead of silently renaming anything (§6).
var epReservedTools = map[string]bool{
	"evidra_prescribe": true,
	"evidra_report":    true,
}

// epProtocolInstructions is the agent-facing contract, derived from the minimal
// instruction set in §9. It is composed with — never replacing — upstream
// instructions (§27 source boundaries).
const epProtocolInstructions = `This MCP server is Evidra, an execution-evidence recorder wrapping one upstream server.

Before using any operational tool from this server, call evidra_prescribe with the objective you intend to achieve.

When the operation is finished, failed, cancelled, or intentionally abandoned, call evidra_report exactly once.

Only one operation may be open in this MCP session. If evidra_prescribe reports operation_already_open, either continue the current operation or call evidra_prescribe again with abandon_and_replace=true.

Tools whose names start with evidra_ are Evidra's own; every other tool is the upstream server's, unchanged.`

// epTool is an MCP tool definition kept as wire data so that annotation
// presence and absence survive the merge (§26, §37).
type epTool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Annotations map[string]any  `json:"annotations,omitempty"`
	Meta        map[string]any  `json:"_meta,omitempty"`
}

// epClientCapabilities are the client-side capabilities the endpoint names when
// it initializes the upstream. tools because the endpoint composes the tool
// list; roots because server-to-client requests are relayed and covered by a
// conformance test. Sampling and elicitation are deliberately not claimed:
// nothing answers them end to end yet.
func epClientCapabilities() map[string]any {
	return map[string]any{
		"tools": map[string]any{},
		"roots": map[string]any{"listChanged": false},
	}
}

// composeInitializeResult builds the client-facing initialize result: the
// supported profile only, advertise-down for anything untested (§27).
func (e *epEndpoint) composeInitializeResult(req *epMsg) (any, *epError) {
	var params struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
	}
	_ = json.Unmarshal(req.Params, &params)

	caps := map[string]any{
		"tools": map[string]any{"listChanged": true},
	}
	if upstream, ok := e.upInit["capabilities"].(map[string]any); ok {
		if _, has := upstream["logging"]; has {
			caps["logging"] = map[string]any{}
		}
		if e.passthrough {
			for _, fam := range []string{"prompts", "resources", "completions"} {
				if v, has := upstream[fam]; has {
					caps[fam] = v
				}
			}
		}
	}

	version := epLatestProtocolVersion
	if params.ProtocolVersion != "" {
		version = epNegotiateVersion(params.ProtocolVersion)
	}

	return map[string]any{
		"protocolVersion": version,
		"capabilities":    caps,
		"serverInfo": map[string]any{
			"name":    "evidra-mcp",
			"title":   "Evidra + " + e.serverName,
			"version": epVersionOf(),
		},
		"instructions": e.composedInstructions(),
	}, nil
}

// epNegotiateVersion answers with the client's requested version when this
// endpoint speaks it, otherwise with its newest supported version.
func epNegotiateVersion(requested string) string {
	for _, v := range epSupportedProtocolVersions {
		if v == requested {
			return requested
		}
	}
	return epLatestProtocolVersion
}

func (e *epEndpoint) composedInstructions() string {
	if !e.upInstrSet {
		return epProtocolInstructions
	}
	return epProtocolInstructions +

		"\n\n--- upstream server instructions (" + e.serverName + ") ---\n" + e.upInstr
}

// prefetchUpstreamTools walks every tools/list page at startup to build the
// merge cache and to enforce the reserved-name rule before serving anything.
func (e *epEndpoint) prefetchUpstreamTools(ctx context.Context) error {
	tools, ann, err := e.listAllUpstreamTools(ctx)
	if err != nil {
		return err
	}
	e.stateMu.Lock()
	e.upTools = tools
	e.upAnn = ann
	e.stateMu.Unlock()
	return nil
}

// refreshUpstreamTools re-runs the walk after an upstream list_changed
// notification. A failed refresh keeps the previous cache rather than leaving
// the client with an empty tool list.
func (e *epEndpoint) refreshUpstreamTools(ctx context.Context) {
	tools, ann, err := e.listAllUpstreamTools(ctx)
	if err != nil {
		e.logger.Printf("endpoint: tools refresh failed, keeping previous list: %v", err)
		return
	}
	e.stateMu.Lock()
	e.upTools = tools
	e.upAnn = ann
	e.stateMu.Unlock()
}

// listAllUpstreamTools follows nextCursor to exhaustion. Cursors are the
// upstream's own opaque values and are never rewritten.
func (e *epEndpoint) listAllUpstreamTools(ctx context.Context) ([]epTool, map[string]map[string]any, error) {
	var (
		tools []epTool
		ann   = map[string]map[string]any{}
		page  int
	)
	cursor := ""
	for {
		page++
		if page > 200 {
			return nil, nil, errors.New("endpoint: upstream tools/list did not terminate after 200 pages")
		}
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := e.requestUpstream(ctx, "tools/list", params, 20*1000*1000*1000)
		if err != nil {
			return nil, nil, fmt.Errorf("endpoint: upstream tools/list: %w", err)
		}
		var result struct {
			Tools      []epTool `json:"tools"`
			NextCursor string   `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			return nil, nil, fmt.Errorf("endpoint: upstream tools/list result: %w", err)
		}
		for _, t := range result.Tools {
			if epReservedTools[t.Name] {
				return nil, nil, fmt.Errorf("endpoint: upstream advertises reserved tool %q; refusing to wrap it (§6: local protocol tools are never renamed)", t.Name)
			}
			tools = append(tools, t)
			if t.Annotations != nil {
				ann[t.Name] = t.Annotations
			}
		}
		if result.NextCursor == "" {
			return tools, ann, nil
		}
		cursor = result.NextCursor
	}
}

// composeToolsListResponse merges the local protocol tools into an upstream
// tools/list result. Per §27 the local tools appear on the first page only and
// the upstream cursor survives untouched, so a client that paginates sees each
// name exactly once.
func (e *epEndpoint) composeToolsListResponse(fwd *epFwd, msg *epMsg) ([]byte, error) {
	if !fwd.firstPage {
		return msg.raw, nil
	}
	raw := msg.Result
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var whole map[string]json.RawMessage
	if err := json.Unmarshal(raw, &whole); err != nil {
		return nil, err
	}
	var upstreamTools []json.RawMessage
	if t := whole["tools"]; len(t) > 0 && string(t) != "null" {
		if err := json.Unmarshal(t, &upstreamTools); err != nil {
			return nil, err
		}
	}
	e.rememberAnnotations(whole["tools"])

	localRaw, err := json.Marshal(epLocalTools)
	if err != nil {
		return nil, err
	}
	var localTools []json.RawMessage
	if err := json.Unmarshal(localRaw, &localTools); err != nil {
		return nil, err
	}
	merged, err := json.Marshal(append(localTools, upstreamTools...))
	if err != nil {
		return nil, err
	}
	whole["tools"] = merged
	out, err := epEncode(&epMsg{JSONRPC: "2.0", ID: msg.ID, Result: epMustMarshal(whole)})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// rememberAnnotations keeps declared annotation objects from a tools payload so
// §26 can later tell declared-false from absent. Reserved local names are
// skipped: they were never declared by the upstream.
func (e *epEndpoint) rememberAnnotations(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var tools []epTool
	if err := json.Unmarshal(raw, &tools); err != nil {
		return
	}
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	for _, t := range tools {
		if epReservedTools[t.Name] {
			continue
		}
		if t.Annotations != nil {
			e.upAnn[t.Name] = t.Annotations
		} else {
			delete(e.upAnn, t.Name)
		}
	}
}

func epMustMarshal(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return []byte(`null`)
	}
	return raw
}

func epVersionOf() string {
	return epBuildVersion
}

// epBuildVersion is the endpoint's contribution to serverInfo. It tracks the
// current development line rather than the released server version, because
// the merged endpoint behaves differently from the legacy direct server.
const epBuildVersion = "core-0"
