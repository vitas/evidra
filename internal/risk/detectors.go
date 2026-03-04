package risk

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"

	"go.yaml.in/yaml/v3"
)

// MassDeleteThreshold is the default count above which mass delete is flagged.
const MassDeleteThreshold = 10

// RunAll runs all detectors against the raw artifact and returns combined risk tags.
func RunAll(raw []byte) []string {
	var tags []string
	for _, d := range []func([]byte) []string{
		DetectPrivileged,
		DetectHostNamespace,
		DetectHostPath,
		DetectMassDestroy,
		DetectWildcardIAM,
		DetectTerraformIAMWildcard,
		DetectS3PublicAccess,
	} {
		tags = append(tags, d(raw)...)
	}
	return tags
}

// --- K8s detectors ---

// DetectPrivileged returns "k8s.privileged_container" if any container has
// securityContext.privileged == true.
func DetectPrivileged(raw []byte) []string {
	for _, obj := range parseK8sYAML(raw) {
		for _, c := range getAllContainers(obj) {
			sc, ok := c["securityContext"].(map[string]interface{})
			if !ok {
				continue
			}
			if priv, ok := sc["privileged"].(bool); ok && priv {
				return []string{"k8s.privileged_container"}
			}
		}
	}
	return nil
}

// DetectHostNamespace returns "k8s.host_namespace_escape" if any pod spec
// uses hostPID, hostIPC, or hostNetwork.
func DetectHostNamespace(raw []byte) []string {
	for _, obj := range parseK8sYAML(raw) {
		spec := getPodSpec(obj)
		if spec == nil {
			continue
		}
		if getBool(spec, "hostPID") || getBool(spec, "hostIPC") || getBool(spec, "hostNetwork") {
			return []string{"k8s.host_namespace_escape"}
		}
	}
	return nil
}

// DetectHostPath returns "k8s.hostpath_mount" if any pod spec has a
// hostPath volume.
func DetectHostPath(raw []byte) []string {
	for _, obj := range parseK8sYAML(raw) {
		spec := getPodSpec(obj)
		if spec == nil {
			continue
		}
		volumes, ok := spec["volumes"].([]interface{})
		if !ok {
			continue
		}
		for _, v := range volumes {
			vol, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			if _, ok := vol["hostPath"]; ok {
				return []string{"k8s.hostpath_mount"}
			}
		}
	}
	return nil
}

// --- Terraform detectors ---

// DetectMassDestroy returns "ops.mass_delete" if a Terraform plan has more
// than MassDeleteThreshold delete actions, or if a K8s multi-doc YAML has
// more than MassDeleteThreshold documents.
func DetectMassDestroy(raw []byte) []string {
	if plan := parsePlan(raw); plan != nil {
		deleteCount := 0
		for _, rc := range plan.ResourceChanges {
			if rc.Change == nil {
				continue
			}
			for _, a := range rc.Change.Actions {
				if a == "delete" {
					deleteCount++
					break
				}
			}
		}
		if deleteCount > MassDeleteThreshold {
			return []string{"ops.mass_delete"}
		}
		return nil
	}

	// Fallback: K8s multi-doc YAML
	if objects := parseK8sYAML(raw); len(objects) > MassDeleteThreshold {
		return []string{"ops.mass_delete"}
	}
	return nil
}

// DetectWildcardIAM returns "aws_iam.wildcard_policy" if any IAM statement
// has both Action:"*" and Resource:"*" (effective root).
func DetectWildcardIAM(raw []byte) []string {
	for _, s := range extractIAMStatements(raw) {
		if s.Effect == "Allow" && s.Action == "*" && s.Resource == "*" {
			return []string{"aws_iam.wildcard_policy"}
		}
	}
	return nil
}

// DetectTerraformIAMWildcard returns "terraform.iam_wildcard_policy" if any
// IAM statement has Action:"*" or Resource:"*".
func DetectTerraformIAMWildcard(raw []byte) []string {
	for _, s := range extractIAMStatements(raw) {
		if s.Effect == "Allow" && (s.Action == "*" || s.Resource == "*") {
			return []string{"terraform.iam_wildcard_policy"}
		}
	}
	return nil
}

// DetectS3PublicAccess returns "terraform.s3_public_access" if a Terraform
// plan creates an S3 bucket without a complete Block Public Access config.
func DetectS3PublicAccess(raw []byte) []string {
	plan := parsePlan(raw)
	if plan == nil {
		return nil
	}

	hasBucket := false
	for _, rc := range plan.ResourceChanges {
		if rc.Type == "aws_s3_bucket" {
			hasBucket = true
		}
	}
	if !hasBucket {
		return nil
	}

	for _, rc := range plan.ResourceChanges {
		if rc.Type == "aws_s3_bucket_public_access_block" && rc.Change != nil {
			if isCompletePublicAccessBlock(rc.Change.After) {
				return nil
			}
		}
	}
	return []string{"terraform.s3_public_access"}
}

// --- K8s helpers ---

func parseK8sYAML(raw []byte) []map[string]interface{} {
	var objects []map[string]interface{}
	decoder := yaml.NewDecoder(bufio.NewReader(bytes.NewReader(raw)))
	for {
		var obj map[string]interface{}
		if err := decoder.Decode(&obj); err == io.EOF {
			break
		} else if err != nil || obj == nil {
			break
		}
		objects = append(objects, obj)
	}
	return objects
}

func getPodSpec(obj map[string]interface{}) map[string]interface{} {
	spec, ok := obj["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	// Deployment/StatefulSet/DaemonSet: spec.template.spec
	if template, ok := spec["template"].(map[string]interface{}); ok {
		if tspec, ok := template["spec"].(map[string]interface{}); ok {
			return tspec
		}
	}
	return spec
}

func getAllContainers(obj map[string]interface{}) []map[string]interface{} {
	spec := getPodSpec(obj)
	if spec == nil {
		return nil
	}
	var containers []map[string]interface{}
	for _, key := range []string{"containers", "initContainers"} {
		list, ok := spec[key].([]interface{})
		if !ok {
			continue
		}
		for _, c := range list {
			if cm, ok := c.(map[string]interface{}); ok {
				containers = append(containers, cm)
			}
		}
	}
	return containers
}

func getBool(m map[string]interface{}, key string) bool {
	v, ok := m[key].(bool)
	return ok && v
}

// --- Terraform helpers ---

type tfPlan struct {
	ResourceChanges []tfResourceChange `json:"resource_changes"`
}

type tfResourceChange struct {
	Type   string    `json:"type"`
	Name   string    `json:"name"`
	Change *tfChange `json:"change"`
}

type tfChange struct {
	Actions []string               `json:"actions"`
	After   map[string]interface{} `json:"after"`
}

func parsePlan(raw []byte) *tfPlan {
	var plan tfPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil
	}
	if len(plan.ResourceChanges) == 0 {
		return nil
	}
	return &plan
}

type iamStatement struct {
	Effect   string `json:"Effect"`
	Action   string `json:"Action"`
	Resource string `json:"Resource"`
}

type iamPolicyDoc struct {
	Statement []iamStatement `json:"Statement"`
}

func extractIAMStatements(raw []byte) []iamStatement {
	plan := parsePlan(raw)
	if plan == nil {
		return nil
	}
	var stmts []iamStatement
	for _, rc := range plan.ResourceChanges {
		if !isIAMResourceType(rc.Type) || rc.Change == nil {
			continue
		}
		policyStr, ok := rc.Change.After["policy"].(string)
		if !ok {
			continue
		}
		var doc iamPolicyDoc
		if err := json.Unmarshal([]byte(policyStr), &doc); err != nil {
			continue
		}
		stmts = append(stmts, doc.Statement...)
	}
	return stmts
}

func isIAMResourceType(t string) bool {
	switch t {
	case "aws_iam_policy", "aws_iam_role_policy", "aws_iam_user_policy", "aws_iam_group_policy":
		return true
	}
	return false
}

func isCompletePublicAccessBlock(after map[string]interface{}) bool {
	for _, key := range []string{
		"block_public_acls",
		"ignore_public_acls",
		"block_public_policy",
		"restrict_public_buckets",
	} {
		v, ok := after[key].(bool)
		if !ok || !v {
			return false
		}
	}
	return true
}
