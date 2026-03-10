package main

import (
	"fmt"

	"samebits.com/evidra-benchmark/internal/score"
)

func resolveCommandScoringProfile(explicit string) (score.Profile, error) {
	profile, err := score.ResolveProfile(explicit)
	if err != nil {
		return score.Profile{}, fmt.Errorf("resolve scoring profile: %w", err)
	}
	return profile, nil
}
