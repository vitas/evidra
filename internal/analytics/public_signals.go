package analytics

import (
	"sort"

	"samebits.com/evidra/internal/score"
	"samebits.com/evidra/internal/signal"
)

// PublicSignalNames returns the stable public signal order used by analytics views and API responses.
func PublicSignalNames(profile score.Profile) []string {
	names := signal.RegisteredSignalNames()
	sort.Slice(names, func(i, j int) bool {
		left := profile.Weight(names[i])
		right := profile.Weight(names[j])
		if left == right {
			return names[i] < names[j]
		}
		return left > right
	})
	return names
}
