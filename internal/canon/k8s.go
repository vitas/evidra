package canon

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// K8sAdapter handles kubectl and oc artifacts.
type K8sAdapter struct{}

func (a *K8sAdapter) Name() string { return "k8s/v1" }
func (a *K8sAdapter) CanHandle(tool string) bool {
	return tool == "kubectl" || tool == "oc" || tool == "helm"
}
func (a *K8sAdapter) Canonicalize(tool, operation, environment string, rawArtifact []byte) (CanonResult, error) {
	r := canonicalizeK8s(tool, operation, environment, rawArtifact)
	return r, r.ParseError
}

type identifiedK8sObject struct {
	identity ResourceID
	obj      map[string]interface{}
}

func canonicalizeK8s(tool, operation, environment string, rawArtifact []byte) CanonResult {
	artifactDigest := sha256Hex(rawArtifact)

	objects, err := splitYAMLDocuments(rawArtifact)
	if err != nil {
		return CanonResult{
			ArtifactDigest: artifactDigest,
			CanonVersion:   "k8s/v1",
			ParseError:     fmt.Errorf("canon.k8s: split YAML: %w", err),
		}
	}

	if len(objects) == 0 {
		return CanonResult{
			ArtifactDigest: artifactDigest,
			CanonVersion:   "k8s/v1",
			ParseError:     fmt.Errorf("canon.k8s: no objects found"),
		}
	}

	var identified []identifiedK8sObject
	for _, obj := range objects {
		id := extractK8sIdentity(obj)
		identified = append(identified, identifiedK8sObject{identity: id, obj: obj})
	}

	sort.Slice(identified, func(i, j int) bool {
		a, b := identified[i].identity, identified[j].identity
		if a.APIVersion != b.APIVersion {
			return a.APIVersion < b.APIVersion
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name
	})

	identities := make([]ResourceID, len(identified))
	for i, id := range identified {
		identities[i] = id.identity
	}

	shapeHash := computeK8sShapeHash(identified)
	opClass := k8sOperationClass(operation)
	scopeClass := ResolveScopeClass(environment, identities)

	action := CanonicalAction{
		Tool:              tool,
		Operation:         operation,
		OperationClass:    opClass,
		ResourceIdentity:  identities,
		ScopeClass:        scopeClass,
		ResourceCount:     len(objects),
		ResourceShapeHash: shapeHash,
	}

	actionJSON, _ := json.Marshal(action)
	intentDigest := ComputeIntentDigest(action)

	return CanonResult{
		ArtifactDigest:  artifactDigest,
		IntentDigest:    intentDigest,
		CanonicalAction: action,
		CanonVersion:    "k8s/v1",
		RawAction:       actionJSON,
	}
}

// splitYAMLDocuments splits a multi-document YAML into individual object maps.
func splitYAMLDocuments(data []byte) ([]map[string]interface{}, error) {
	var objects []map[string]interface{}
	reader := bufio.NewReader(bytes.NewReader(data))
	decoder := yaml.NewDecoder(reader)

	for {
		var obj map[string]interface{}
		err := decoder.Decode(&obj)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode YAML document: %w", err)
		}
		if obj == nil {
			continue
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

// extractK8sIdentity pulls apiVersion, kind, namespace, name from a K8s object.
func extractK8sIdentity(obj map[string]interface{}) ResourceID {
	id := ResourceID{}

	if v, ok := obj["apiVersion"].(string); ok {
		id.APIVersion = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := obj["kind"].(string); ok {
		id.Kind = strings.ToLower(strings.TrimSpace(v))
	}

	if metadata, ok := obj["metadata"].(map[string]interface{}); ok {
		if v, ok := metadata["namespace"].(string); ok {
			id.Namespace = strings.ToLower(strings.TrimSpace(v))
		}
		if v, ok := metadata["name"].(string); ok {
			id.Name = strings.ToLower(strings.TrimSpace(v))
		}
	}

	return id
}

// computeK8sShapeHash computes SHA256 over the noise-removed, sorted objects.
func computeK8sShapeHash(objects []identifiedK8sObject) string {
	cleaned := make([]map[string]interface{}, len(objects))
	for i, o := range objects {
		copied := deepCopyMap(o.obj)
		removeK8sNoiseFields(copied)
		cleaned[i] = copied
	}

	data, err := json.Marshal(cleaned)
	if err != nil {
		return ""
	}

	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// deepCopyMap creates a deep copy of a map[string]interface{}.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = deepCopyMap(val)
		case []interface{}:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

func deepCopySlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]interface{}:
			result[i] = deepCopyMap(val)
		case []interface{}:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}
