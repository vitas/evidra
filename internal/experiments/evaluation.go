package experiments

type ArtifactEvaluation struct {
	PredictedRiskLevel   string   `json:"predicted_risk_level"`
	PredictedRiskDetails []string `json:"predicted_risk_details"`
	RiskLevelMatch       bool     `json:"risk_level_match"`
	TruePositive         int      `json:"true_positive"`
	FalsePositive        int      `json:"false_positive"`
	FalseNegative        int      `json:"false_negative"`
	Precision            *float64 `json:"precision"`
	Recall               *float64 `json:"recall"`
	F1                   *float64 `json:"f1"`
}

func evaluateArtifact(expectedLevel, predictedLevel string, expectedTags, predictedTags []string) ArtifactEvaluation {
	eTags := normalizeTags(expectedTags)
	pTags := normalizeTags(predictedTags)
	tp, fp, fn := overlapCounts(eTags, pTags)
	precision := safeDiv(float64(tp), float64(tp+fp))
	recall := safeDiv(float64(tp), float64(tp+fn))
	f1 := calcF1(precision, recall)

	return ArtifactEvaluation{
		PredictedRiskLevel:   predictedLevel,
		PredictedRiskDetails: pTags,
		RiskLevelMatch:       expectedLevel != "" && predictedLevel != "" && expectedLevel == predictedLevel,
		TruePositive:         tp,
		FalsePositive:        fp,
		FalseNegative:        fn,
		Precision:            precision,
		Recall:               recall,
		F1:                   f1,
	}
}

type ExecutionEvaluation struct {
	PrescribeOK     bool     `json:"prescribe_ok"`
	ReportOK        bool     `json:"report_ok"`
	ProtocolOK      bool     `json:"protocol_ok"`
	CommandExitCode *int     `json:"command_exit_code"`
	ExitCodeMatch   bool     `json:"exit_code_match"`
	ObservedLevel   string   `json:"observed_risk_level"`
	ObservedTags    []string `json:"observed_risk_tags"`
	RiskLevelMatch  *bool    `json:"risk_level_match"`
	TruePositive    int      `json:"true_positive"`
	FalsePositive   int      `json:"false_positive"`
	FalseNegative   int      `json:"false_negative"`
	Precision       *float64 `json:"precision"`
	Recall          *float64 `json:"recall"`
	F1              *float64 `json:"f1"`
	Pass            bool     `json:"pass"`
}

func evaluateExecution(
	prescribeOK, reportOK bool,
	expectedExit, observedExit *int,
	expectedLevel, observedLevel string,
	expectedTags, observedTags []string,
) ExecutionEvaluation {
	eTags := normalizeTags(expectedTags)
	oTags := normalizeTags(observedTags)
	tp, fp, fn := overlapCounts(eTags, oTags)
	precision := safeDiv(float64(tp), float64(tp+fp))
	recall := safeDiv(float64(tp), float64(tp+fn))
	f1 := calcF1(precision, recall)

	var riskLevelMatch *bool
	if expectedLevel != "" {
		match := expectedLevel == observedLevel
		riskLevelMatch = &match
	}

	exitMatch := true
	if expectedExit != nil {
		exitMatch = observedExit != nil && *expectedExit == *observedExit
	}
	levelPass := true
	if riskLevelMatch != nil {
		levelPass = *riskLevelMatch
	}
	pass := prescribeOK && reportOK && exitMatch && levelPass

	return ExecutionEvaluation{
		PrescribeOK:     prescribeOK,
		ReportOK:        reportOK,
		ProtocolOK:      prescribeOK && reportOK,
		CommandExitCode: observedExit,
		ExitCodeMatch:   exitMatch,
		ObservedLevel:   observedLevel,
		ObservedTags:    oTags,
		RiskLevelMatch:  riskLevelMatch,
		TruePositive:    tp,
		FalsePositive:   fp,
		FalseNegative:   fn,
		Precision:       precision,
		Recall:          recall,
		F1:              f1,
		Pass:            pass,
	}
}

func overlapCounts(expected, observed []string) (tp, fp, fn int) {
	expSet := make(map[string]struct{}, len(expected))
	obsSet := make(map[string]struct{}, len(observed))
	for _, v := range expected {
		expSet[v] = struct{}{}
	}
	for _, v := range observed {
		obsSet[v] = struct{}{}
	}
	for v := range obsSet {
		if _, ok := expSet[v]; ok {
			tp++
		} else {
			fp++
		}
	}
	for v := range expSet {
		if _, ok := obsSet[v]; !ok {
			fn++
		}
	}
	return tp, fp, fn
}

func safeDiv(a, b float64) *float64 {
	if b == 0 {
		return nil
	}
	v := a / b
	return &v
}

func calcF1(precision, recall *float64) *float64 {
	if precision == nil || recall == nil {
		return nil
	}
	sum := *precision + *recall
	if sum == 0 {
		return nil
	}
	v := 2 * (*precision) * (*recall) / sum
	return &v
}
