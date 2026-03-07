package experiments

type ArtifactRunOptions struct {
	ModelID        string
	Provider       string
	PromptVersion  string
	PromptFile     string
	Temperature    *float64
	Mode           string
	Repeats        int
	TimeoutSeconds int
	CaseFilter     string
	MaxCases       int
	CasesDir       string
	OutDir         string
	CleanOutDir    bool
	Agent          string
	DryRun         bool
}

type ExecutionRunOptions struct {
	ModelID        string
	Provider       string
	PromptVersion  string
	PromptFile     string
	ScenariosDir   string
	Mode           string
	Repeats        int
	TimeoutSeconds int
	ScenarioFilter string
	MaxScenarios   int
	OutDir         string
	CleanOutDir    bool
	Agent          string
	DryRun         bool
}

type ArtifactCase struct {
	CaseID              string
	Category            string
	Difficulty          string
	GroundTruthPattern  string
	ExpectedRiskLevel   string
	ExpectedRiskDetails []string
	ArtifactPath        string
	ExpectedJSONPath    string
}

type ExecutionScenario struct {
	ScenarioID        string
	Category          string
	Difficulty        string
	Tool              string
	Operation         string
	ArtifactPath      string
	ExecuteCommand    string
	ExpectedExitCode  *int
	ExpectedRiskLevel string
	ExpectedRiskTags  []string
	SourceJSONPath    string
}

type ArtifactAgentRequest struct {
	Case            ArtifactCase
	ModelID         string
	Provider        string
	Prompt          PromptInfo
	Temperature     *float64
	TimeoutSeconds  int
	RunID           string
	RepeatIndex     int
	RawStreamOut    string
	AgentOutputPath string
}

type ArtifactAgentResult struct {
	Output    map[string]any
	StdoutLog string
	StderrLog string
	RawStream string
}

type ExecutionAgentRequest struct {
	Scenario        ExecutionScenario
	ModelID         string
	Provider        string
	Prompt          PromptInfo
	TimeoutSeconds  int
	RunID           string
	RepeatIndex     int
	RawStreamOut    string
	AgentOutputPath string
}

type ExecutionAgentResult struct {
	Output    map[string]any
	StdoutLog string
	StderrLog string
	RawStream string
}

type RunCounters struct {
	Total    int
	Success  int
	Failure  int
	Timeout  int
	DryRun   int
	EvalPass int
	EvalFail int
}
