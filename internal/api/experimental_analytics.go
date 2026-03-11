package api

import "errors"

var ErrExperimentalAnalytics = errors.New("hosted analytics are experimental")

const experimentalAnalyticsMessage = "hosted analytics are experimental; use CLI/MCP for authoritative analytics"

// ExperimentalAnalytics keeps self-hosted analytics routes explicit until parity exists.
type ExperimentalAnalytics struct{}

func (ExperimentalAnalytics) ComputeScorecard(string, AnalyticsFilters) (interface{}, error) {
	return nil, ErrExperimentalAnalytics
}

func (ExperimentalAnalytics) ComputeExplain(string, AnalyticsFilters) (interface{}, error) {
	return nil, ErrExperimentalAnalytics
}
