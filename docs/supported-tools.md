# Supported Tools

- Status: Reference
- Version: current
- Canonical for: supported tool matrix and adapter coverage
- Audience: public

Evidra canonicalizes artifacts from the following tools via built-in adapters.

## Kubernetes (k8s/v1)

| Tool | CLI flag | Artifact | Notes |
|---|---|---|---|
| kubectl | `--tool kubectl` | YAML manifest(s) | Multi-doc YAML supported |
| Helm | `--tool helm` | Rendered templates (`helm template` output) | K8s adapter parses the YAML |
| Kustomize | `--tool kustomize` | Build output (`kustomize build` output) | K8s adapter parses the YAML |
| OpenShift (oc) | `--tool oc` | YAML manifest(s) | Handles DeploymentConfig, Route, BuildConfig, ImageStream |
| ArgoCD | `--tool kubectl` | Rendered sync manifests | Use kubectl; ArgoCD-specific adapter planned for v0.5.0 |

**Noise filtering:** managedFields, uid, resourceVersion, creationTimestamp, last-applied-configuration, and other server-set fields are stripped before canonicalization.

**Risk detectors:** privileged containers, hostNetwork/hostPID, hostPath mounts, docker socket mounts, dangerous capabilities, cluster-admin RBAC, writable root filesystem, run-as-root.

## Terraform (terraform/v1)

| Tool | CLI flag | Artifact | Notes |
|---|---|---|---|
| Terraform | `--tool terraform` | Plan JSON (`terraform show -json`) | Extracts resource_changes |

**Artifact preparation:**
```bash
terraform plan -out=tfplan
terraform show -json tfplan > plan.json
```

**Risk detectors:** world-open security groups (0.0.0.0/0), public S3 buckets, IAM wildcard policies, unencrypted EBS volumes, public RDS instances.

## Docker (docker/v1)

| Tool | CLI flag | Artifact | Notes |
|---|---|---|---|
| Docker | `--tool docker` | Container inspect JSON | `docker inspect` output |

**Risk detectors:** privileged mode, host network, docker socket mounts.

## Generic fallback (generic/v1)

Any tool not listed above falls through to the generic adapter. It computes artifact digest and basic operation metadata but cannot extract resource-level identity or run domain-specific risk detectors.

For tools with structured output (Pulumi, Ansible, CDK), use `--canonical-action` to provide pre-built resource identity and bypass the adapter:

```bash
evidra prescribe \
  --tool pulumi \
  --operation update \
  --artifact state.json \
  --canonical-action '{"resource_identity": [...], "resource_count": 2, "operation_class": "mutate"}'
```

## Adding tool support

Adapters implement the `canon.Adapter` interface:

```go
type Adapter interface {
    Name() string
    CanHandle(tool string) bool
    Canonicalize(tool, operation, environment string, rawArtifact []byte) (CanonResult, error)
}
```

See `internal/canon/` for existing adapter implementations.
