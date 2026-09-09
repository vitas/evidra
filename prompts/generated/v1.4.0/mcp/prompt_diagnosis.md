<!-- contract: v1.4.0 -->
# Infrastructure Diagnosis Flowchart

Follow this protocol when investigating infrastructure issues before making changes.

## Step 1: Gather Context

```
kubectl get events -n <namespace> --sort-by='.lastTimestamp'
kubectl get pods -n <namespace>
kubectl describe <resource> -n <namespace>
```

Look for: CrashLoopBackOff, ImagePullBackOff, OOMKilled, FailedScheduling, Unhealthy

## Step 2: Check Logs

```
kubectl logs <pod> -n <namespace> --tail=50
kubectl logs <pod> -n <namespace> --previous    # if crashed
```

## Step 3: Identify Root Cause

Common patterns:
- **CrashLoopBackOff: Check logs for application errors, missing config, wrong command**
- **ImagePullBackOff: Wrong image name/tag, missing pull secret, private registry**
- **Pending: Insufficient resources, node selector mismatch, PVC not bound**
- **OOMKilled: Container memory limit too low for workload**
- **FailedMount: Missing ConfigMap, Secret, or PV**

## Step 4: Fix (One Change)

Make exactly one targeted fix. Use `run_command` by default; switch to explicit prescribe/report only when you need tighter control.

```
investigate -> run_command mutation (auto-evidence) -> verify
```

## Step 5: Verify

```
kubectl rollout status deployment/<name> -n <namespace> --timeout=60s
kubectl get pods -n <namespace>
```

## Rules

- Never skip investigation — always describe/events/logs before patching
- One fix at a time — verify before making another change
- Don't delete and recreate when a patch suffices
- Don't fix things that aren't broken
