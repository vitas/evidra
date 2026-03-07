package docker

import (
	"strings"

	"go.yaml.in/yaml/v3"
)

type composeFile struct {
	Services map[string]map[string]interface{} `yaml:"services"`
}

func parseCompose(raw []byte) composeFile {
	var cf composeFile
	_ = yaml.Unmarshal(raw, &cf) // best-effort: returns empty struct on invalid YAML
	return cf
}

func hasDockerSockMount(vol interface{}) bool {
	switch v := vol.(type) {
	case string:
		return strings.Contains(strings.ToLower(v), "docker.sock")
	case map[string]interface{}:
		for _, k := range []string{"source", "target", "type"} {
			if s, ok := v[k].(string); ok && strings.Contains(strings.ToLower(s), "docker.sock") {
				return true
			}
		}
	}
	return false
}
