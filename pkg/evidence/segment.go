package evidence

import (
	"fmt"
	"path/filepath"
	"sort"
)

func orderedSegmentNames(root string) ([]int, []string, error) {
	segDir := filepath.Join(root, segmentsDirName)
	matches, err := filepath.Glob(filepath.Join(segDir, "evidence-*.jsonl"))
	if err != nil {
		return nil, nil, err
	}
	if len(matches) == 0 {
		return nil, nil, nil
	}

	names := make([]string, 0, len(matches))
	indices := make([]int, 0, len(matches))
	for _, m := range matches {
		name := filepath.Base(m)
		idx, err := parseSegmentIndex(name)
		if err != nil {
			return nil, nil, err
		}
		names = append(names, name)
		indices = append(indices, idx)
	}

	sort.SliceStable(names, func(i, j int) bool { return names[i] < names[j] })
	sort.Ints(indices)

	for i, idx := range indices {
		expected := i + 1
		if idx != expected {
			return nil, nil, fmt.Errorf("missing segment in sequence: expected %s", segmentName(expected))
		}
	}

	for i, name := range names {
		expected := segmentName(i + 1)
		if name != expected {
			return nil, nil, fmt.Errorf("unexpected segment name: %s", name)
		}
	}

	return indices, names, nil
}

func parseSegmentIndex(name string) (int, error) {
	var idx int
	n, err := fmt.Sscanf(name, "evidence-%06d.jsonl", &idx)
	if err != nil || n != 1 || idx <= 0 {
		return 0, fmt.Errorf("invalid segment filename: %s", name)
	}
	if name != segmentName(idx) {
		return 0, fmt.Errorf("invalid segment filename: %s", name)
	}
	return idx, nil
}

func segmentName(idx int) string {
	return fmt.Sprintf("evidence-%06d.jsonl", idx)
}

func normalizeSealedSegments(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func removeSegment(in []string, segment string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == segment {
			continue
		}
		out = append(out, s)
	}
	return out
}
