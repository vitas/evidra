package analytics

// Filters selects the evidence set used for self-hosted analytics.
type Filters struct {
	Period        string
	Actor         string
	Tool          string
	Scope         string
	SessionID     string
	MinOperations int
}
