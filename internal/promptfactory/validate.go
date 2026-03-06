package promptfactory

import (
	"fmt"
)

func ValidateBundle(bundle Bundle, expectedVersion string) error {
	if err := validateContract(bundle.Contract, expectedVersion); err != nil {
		return err
	}
	if err := validateMCPContract(bundle.Contract.MCP); err != nil {
		return err
	}
	if err := validateLiteLLMContract(bundle.Contract.LiteLLM); err != nil {
		return err
	}
	if err := validateClassification(bundle.Classification); err != nil {
		return err
	}
	if err := validateOutputContracts(bundle.Output); err != nil {
		return err
	}
	if err := validateAgentContract(bundle.Contract.AgentContract); err != nil {
		return err
	}
	return nil
}

func validateContract(contract Contract, expectedVersion string) error {
	if contract.Version == "" {
		return fmt.Errorf("contract.contract_version is required")
	}
	if expectedVersion != "" && contract.Version != expectedVersion {
		return fmt.Errorf("contract version mismatch: bundle=%s expected=%s", contract.Version, expectedVersion)
	}
	if len(contract.Invariants) == 0 {
		return fmt.Errorf("contract.invariants is required")
	}
	return nil
}

func validateMCPContract(mcp MCPContract) error {
	if len(mcp.Initialize.ProductSummary) == 0 {
		return fmt.Errorf("mcp.initialize.product_summary is required")
	}
	if mcp.Initialize.ProtocolIntro == "" {
		return fmt.Errorf("mcp.initialize.protocol_intro is required")
	}
	if len(mcp.Initialize.CriticalInvariants) == 0 {
		return fmt.Errorf("mcp.initialize.critical_invariants is required")
	}
	if len(mcp.Initialize.Rules) == 0 {
		return fmt.Errorf("mcp.initialize.rules is required")
	}
	if mcp.Prescribe.Intro == "" {
		return fmt.Errorf("mcp.prescribe.intro is required")
	}
	if len(mcp.Prescribe.RequiredInputs) == 0 {
		return fmt.Errorf("mcp.prescribe.required_inputs is required")
	}
	if len(mcp.Prescribe.PreCallChecklist) == 0 {
		return fmt.Errorf("mcp.prescribe.pre_call_checklist is required")
	}
	if len(mcp.Report.RequiredInputs) == 0 {
		return fmt.Errorf("mcp.report.required_inputs is required")
	}
	if len(mcp.Report.TerminalOutcomeRule) == 0 {
		return fmt.Errorf("mcp.report.terminal_outcome_rule is required")
	}
	if len(mcp.Report.Rules) == 0 {
		return fmt.Errorf("mcp.report.rules is required")
	}
	if mcp.GetEvent.Intro == "" {
		return fmt.Errorf("mcp.get_event.intro is required")
	}
	if len(mcp.GetEvent.Input) == 0 {
		return fmt.Errorf("mcp.get_event.input is required")
	}
	if len(mcp.GetEvent.Returns) == 0 {
		return fmt.Errorf("mcp.get_event.returns is required")
	}
	return nil
}

func validateLiteLLMContract(contract LiteLLMContract) error {
	if len(contract.SystemIntro) == 0 {
		return fmt.Errorf("litellm.system_intro is required")
	}
	if len(contract.ExecutionModeRules) == 0 {
		return fmt.Errorf("litellm.execution_mode_rules is required")
	}
	if len(contract.AssessmentModeRequirements) == 0 {
		return fmt.Errorf("litellm.assessment_mode_requirements is required")
	}
	return nil
}

func validateClassification(classification Classification) error {
	if len(classification.MutateExamples) == 0 {
		return fmt.Errorf("classification.mutate_examples is required")
	}
	if len(classification.ReadOnlyExamples) == 0 {
		return fmt.Errorf("classification.read_only_examples is required")
	}
	return nil
}

func validateOutputContracts(output OutputContracts) error {
	if output.AssessmentJSON.LevelField == "" || output.AssessmentJSON.DetailsField == "" {
		return fmt.Errorf("output_contracts.assessment_json fields are required")
	}
	if len(output.AssessmentJSON.AllowedLevel) == 0 {
		return fmt.Errorf("output_contracts.assessment_json.allowed_levels is required")
	}
	return nil
}

func validateAgentContract(contract AgentContract) error {
	if contract.Title == "" {
		return fmt.Errorf("agent_contract.title is required")
	}
	if contract.VersionPolicy == "" {
		return fmt.Errorf("agent_contract.version_policy is required")
	}
	if len(contract.Changelog) == 0 {
		return fmt.Errorf("agent_contract.changelog is required")
	}
	if len(contract.ExecutionRules) == 0 {
		return fmt.Errorf("agent_contract.execution_rules is required")
	}
	if len(contract.OutputRules) == 0 {
		return fmt.Errorf("agent_contract.output_rules is required")
	}
	return nil
}
