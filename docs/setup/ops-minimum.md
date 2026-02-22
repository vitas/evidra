# Ops Minimum (Argo CD Evaluation)

This page defines the minimum operational setup for evaluating Evidra safely in a real Argo CD environment.

## 1) Minimal permissions (read-only)

Evidra collects Argo CD application state using a **Kubernetes dynamic informer** as the primary path. The Argo CD API is used as a fallback when no Kubernetes credentials are available.

### Kubernetes RBAC (primary path — in-cluster or kubeconfig)

Evidra requires read access to `argoproj.io/v1alpha1/applications` in the namespace where Argo CD Applications live (typically `argocd`).

Minimal ClusterRole example:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: evidra-argo-reader
rules:
- apiGroups: ["argoproj.io"]
  resources: ["applications"]
  verbs: ["get", "list", "watch"]
```

Bind this role to the Evidra service account with a ClusterRoleBinding (or namespaced RoleBinding if scoped to one namespace).

### Argo CD API token (fallback path)

The Argo CD API URL is still required by configuration (`EVIDRA_ARGO_API_URL`), even when running in-cluster. The API token is used only for the polling fallback path (when Kubernetes config is unavailable).

Recommended approach:
1. Create a dedicated Argo CD account for Evidra (read-only).
2. Bind that account to a read-only Argo CD role.
3. Issue an API token for that account.
4. Store the token in Kubernetes Secret as `EVIDRA_ARGO_API_TOKEN`.

Example Argo CD RBAC policy (for the API token fallback path):

```csv
# policy.csv (example)
p, role:evidra-read, applications, get, */*, allow
p, role:evidra-read, projects, get, *, allow
g, evidra, role:evidra-read
```

Notes:
- Keep this account separate from human admin users.
- Limit visible applications/projects according to your environment policy.

## 2) Token rotation

Operational rotation flow:
1. Generate a new Argo CD token for the Evidra account.
2. Update `EVIDRA_ARGO_API_TOKEN` in `Secret/evidra-secrets`.
3. Restart Evidra deployment (`kubectl rollout restart`).
4. Verify collector recovery in logs and API behavior.

Expected symptoms when token is invalid/expired:
- `401`/`403` errors in collector logs.
- No new changes ingested after the token becomes invalid.
- Existing data remains readable, but freshness stops.

Recommended practice:
- Rotate on a fixed schedule.
- Rotate immediately after account/credential policy changes.
- Keep token lifetime aligned with platform security policy.

## 3) Ingest load and safety

Key Argo ingest controls:
- `EVIDRA_ARGO_COLLECTOR_ENABLED` (`true/false`)
- `EVIDRA_ARGO_API_URL` (Argo API endpoint — required even in Kubernetes; used for the polling fallback)
- `EVIDRA_ARGO_API_TOKEN` (when token auth is used for polling fallback)
- `EVIDRA_ARGO_COLLECTOR_INTERVAL` (polling fallback interval, default `30s`)
- `EVIDRA_ARGO_CHECKPOINT_FILE` (dedup cursor state, default `./var/argo_checkpoint.json`)

Primary path (Kubernetes dynamic informer) load notes:
- The informer subscribes to Application watch events from the Kubernetes API server. Load is event-driven, not periodic.
- Ensure the Evidra service account has the minimal RBAC described in section 1.

Fallback path (polling) load notes:
- Only used when no Kubernetes credentials are available.
- Increase `EVIDRA_ARGO_COLLECTOR_INTERVAL` to reduce Argo CD API polling frequency (for example `60s` or `120s`).
- Point `EVIDRA_ARGO_API_URL` to the in-cluster Argo CD service URL for lower latency.

Safe defaults for evaluation:
- Collector enabled with `30s` fallback interval.
- Read-only Kubernetes service account scoped to Argo CD namespace.
- No cluster-wide collection outside Argo CD Applications.

## 4) Observability minimum

Available endpoints:
- `GET /healthz`
- `GET /metrics` (Prometheus format)

Key log lines:
- Collector start:
  - `argo collector started`
- Connectivity/auth failures:
  - `argo collector fetch error`
  - `argo collector fetch backoff exhausted`

Quick checks:

```bash
# health
curl -fsS http://localhost:8080/healthz

# metrics
curl -fsS http://localhost:8080/metrics | head

# collector logs in Kubernetes
kubectl -n evidra logs deploy/evidra-prod | rg -n "argo collector|fetch error|backoff exhausted"
```

How to confirm recent ingest:
- Check latest change timestamps for expected subjects in Explorer/API.
- Confirm collector errors are not repeating continuously.

Current behavior note:
- v0.1 does not emit a dedicated "poll success" log per cycle.
- Use data freshness + absence of recurring collector errors as operational signal.

## 5) Troubleshooting quick list

### 401/403 from Argo API
- Verify `EVIDRA_ARGO_API_TOKEN`.
- Verify Argo RBAC role bindings for the Evidra account.
- Rotate token and restart deployment.

### Wrong Argo API URL
- Verify `EVIDRA_ARGO_API_URL`.
- In Kubernetes, prefer in-cluster Argo service URL over external ingress URL for collector traffic.

### Timestamp confusion (timezone/clock)
- Ensure node clocks are synchronized.
- Compare using UTC timestamps in API responses and logs.

### No changes visible
- Verify `EVIDRA_ARGO_COLLECTOR_ENABLED=true`.
- Check collector interval (`EVIDRA_ARGO_COLLECTOR_INTERVAL`).
- Check collector errors in logs.
- Confirm Argo application activity exists in the selected time window.

## 6) Related setup docs

- Quickstart: `docs/setup/quickstart.md`
- Kubernetes deployment: `docs/setup/deployment-kustomize.md`
- Argo ingest model: `docs/architecture/argo-ingest-logic.md`

