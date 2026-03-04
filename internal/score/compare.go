package score

// WorkloadProfile describes the tools and scopes an agent operates in.
type WorkloadProfile struct {
	Tools  map[string]bool `json:"tools"`
	Scopes map[string]bool `json:"scopes"`
}

// WorkloadOverlap computes the Jaccard similarity of two workload profiles.
// Returns a value in [0, 1] where 1 means identical profiles.
func WorkloadOverlap(a, b WorkloadProfile) float64 {
	toolOverlap := jaccard(a.Tools, b.Tools)
	scopeOverlap := jaccard(a.Scopes, b.Scopes)
	return toolOverlap * scopeOverlap
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	union := make(map[string]bool)
	for k := range a {
		union[k] = true
	}
	for k := range b {
		union[k] = true
	}
	if len(union) == 0 {
		return 1
	}
	var intersect int
	for k := range a {
		if b[k] {
			intersect++
		}
	}
	return float64(intersect) / float64(len(union))
}
