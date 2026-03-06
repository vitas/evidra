# Evidra Benchmark Dataset - Coverage Report

Generated: 2026-03-06T11:37:19Z

**Dataset label:** `limited-contract-baseline`  
**Dataset scope:** `limited`  
**Cases:** 10 | **Corpus artifacts:** 0

## Signal Coverage

| Signal / Risk Tag | FAIL Cases | PASS Cases | Total |
|-------------------|-----------|-----------|-------|
| `k8s.hostpath_mount` | 1 | 1 | 2 |
| `k8s.privileged_container` | 1 | 1 | 2 |
| `k8s.run_as_root` | 1 | 1 | 2 |
| `terraform.iam_wildcard_policy` | 1 | 0 | 1 |
| `terraform.s3_public_access` | 1 | 0 | 1 |
| `tf.iam_wildcard_policy` | 1 | 1 | 2 |
| `tf.s3_public_access` | 1 | 1 | 2 |

## Gaps (signals with < 2 FAIL cases)

- `k8s.hostpath_mount`: FAIL cases=1
- `k8s.privileged_container`: FAIL cases=1
- `k8s.run_as_root`: FAIL cases=1
- `terraform.iam_wildcard_policy`: FAIL cases=1
- `terraform.s3_public_access`: FAIL cases=1
- `tf.iam_wildcard_policy`: FAIL cases=1
- `tf.s3_public_access`: FAIL cases=1

## By Category

| Category | Cases |
|----------|-------|
| kubernetes | 6 |
| terraform | 4 |

## By Ground Truth Pattern

| Pattern | Cases |
|---------|-------|
| `tf.s3_public_access` | 2 |
| `tf.iam_wildcard_policy` | 2 |
| `k8s.run_as_root` | 2 |
| `k8s.privileged_container` | 2 |
| `k8s.hostpath_mount` | 2 |

## By Difficulty

| Difficulty | Cases | % |
|-----------|-------|---|
| easy | 2 | 20% |
| medium | 6 | 60% |
| hard | 2 | 20% |
| catastrophic | 0 | 0% |
