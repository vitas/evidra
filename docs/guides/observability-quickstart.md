# Observability Quickstart

Evidra exports operation metrics via OTLP/HTTP. Connect any OTLP-compatible backend (Prometheus, Grafana Cloud, Datadog, New Relic, Honeycomb) to get reliability dashboards out of the box.

## Enable Metrics Export

Set two environment variables:

```bash
export EVIDRA_METRICS_TRANSPORT=otlp_http
export EVIDRA_METRICS_OTLP_ENDPOINT=http://localhost:4318/v1/metrics
```

Metrics are emitted on every `evidra run` and `evidra record` invocation. If the endpoint is unreachable, the operation still completes — metrics export is fire-and-forget.

| Variable | Purpose | Default |
|---|---|---|
| `EVIDRA_METRICS_TRANSPORT` | Transport backend (`none` or `otlp_http`) | `none` |
| `EVIDRA_METRICS_OTLP_ENDPOINT` | OTLP/HTTP endpoint URL | (required when `otlp_http`) |
| `EVIDRA_METRICS_TIMEOUT` | HTTP timeout for metrics push | `3s` |

## Metrics Reference

Evidra emits two metrics per operation:

### `evidra.operation.signal.count`

Number of times a behavioral signal fired during the operation. One data point per signal detected, plus a `signal_name=none` point when the operation is clean.

Use this to track signal frequency over time: how often do retries, drift, or blast radius events occur?

### `evidra.operation.duration_ms`

Wall-clock duration of the wrapped command (milliseconds). Only meaningful for `evidra run` (where Evidra executes the command). For `evidra record`, this reflects the duration reported in the input payload.

## Label Reference

All metrics carry six bounded-cardinality labels. Values outside the allowed set are normalized to a fallback (e.g., unknown tool becomes `other`).

| Label | Allowed Values | Fallback |
|---|---|---|
| `tool` | `terraform`, `kubectl`, `helm`, `ansible`, `docker`, `bash`, `argocd`, `github_actions`, `ci` | `other` |
| `environment` | `production`, `staging`, `development` | `unknown` |
| `result_class` | `success`, `failure` | `unknown` |
| `signal_name` | `protocol_violation`, `artifact_drift`, `retry_loop`, `blast_radius`, `new_scope`, `repair_loop`, `thrashing`, `risk_escalation`, `none` | `other` |
| `score_band` | `excellent`, `good`, `fair`, `poor`, `insufficient_data` | `unknown` |
| `assessment_mode` | `preview`, `sufficient` | `unknown` |

Bounded cardinality prevents label explosion in your metrics backend. The maximum label combinations are: 10 tools x 4 environments x 3 result classes x 10 signals x 6 bands x 3 modes = 21,600.

## Collector Setup

### OpenTelemetry Collector

Minimal `otel-collector-config.yaml`:

```yaml
receivers:
  otlp:
    protocols:
      http:
        endpoint: 0.0.0.0:4318

exporters:
  prometheus:
    endpoint: 0.0.0.0:8889

service:
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [prometheus]
```

Run:

```bash
docker run -d --name otel-collector \
  -p 4318:4318 -p 8889:8889 \
  -v $(pwd)/otel-collector-config.yaml:/etc/otelcol/config.yaml \
  otel/opentelemetry-collector-contrib:latest
```

Then point Evidra at it:

```bash
export EVIDRA_METRICS_TRANSPORT=otlp_http
export EVIDRA_METRICS_OTLP_ENDPOINT=http://localhost:4318/v1/metrics
```

### Grafana Alloy

```alloy
otelcol.receiver.otlp "evidra" {
  http {
    listen_address = "0.0.0.0:4318"
  }
  output {
    metrics = [otelcol.exporter.prometheus.default.input]
  }
}

otelcol.exporter.prometheus "default" {
  forward_to = [prometheus.remote_write.default.receiver]
}

prometheus.remote_write "default" {
  endpoint {
    url = "http://prometheus:9090/api/v1/write"
  }
}
```

### Grafana Cloud / Datadog / New Relic

Point `EVIDRA_METRICS_OTLP_ENDPOINT` directly at your vendor's OTLP ingest endpoint. Most vendors accept OTLP/HTTP natively:

```bash
# Grafana Cloud
export EVIDRA_METRICS_OTLP_ENDPOINT=https://otlp-gateway-<region>.grafana.net/otlp/v1/metrics

# Datadog
export EVIDRA_METRICS_OTLP_ENDPOINT=https://http-intake.logs.datadoghq.com/api/v2/otlp/v1/metrics

# New Relic
export EVIDRA_METRICS_OTLP_ENDPOINT=https://otlp.nr-data.net/v1/metrics
```

Refer to your vendor docs for authentication headers — most accept a bearer token or API key via standard OTLP headers.

## Example Queries (PromQL)

### Operations per hour by result

```promql
sum by (result_class) (
  rate(evidra_operation_signal_count{signal_name="none"}[1h])
)
```

### Signal frequency (which signals fire most?)

```promql
topk(5,
  sum by (signal_name) (
    increase(evidra_operation_signal_count{signal_name!="none"}[24h])
  )
)
```

### Failure rate by tool and environment

```promql
sum by (tool, environment) (
  rate(evidra_operation_signal_count{result_class="failure", signal_name="none"}[1h])
)
/
sum by (tool, environment) (
  rate(evidra_operation_signal_count{signal_name="none"}[1h])
)
```

### Score band distribution (last 24h)

```promql
sum by (score_band) (
  increase(evidra_operation_signal_count{signal_name="none"}[24h])
)
```

### Artifact drift trend

```promql
sum(
  increase(evidra_operation_signal_count{signal_name="artifact_drift"}[1d])
)
```

### p95 operation duration by tool

```promql
histogram_quantile(0.95,
  sum by (tool, le) (
    rate(evidra_operation_duration_ms_bucket[1h])
  )
)
```

### Retry loop detection (alert candidate)

```promql
sum by (tool, environment) (
  increase(evidra_operation_signal_count{signal_name="retry_loop"}[1h])
) > 3
```

## CI Integration

### GitHub Actions

```yaml
- name: Setup Evidra
  uses: samebits/evidra/.github/actions/setup-evidra@main

- name: Deploy with observability
  env:
    EVIDRA_METRICS_TRANSPORT: otlp_http
    EVIDRA_METRICS_OTLP_ENDPOINT: ${{ secrets.OTLP_ENDPOINT }}
    EVIDRA_SIGNING_KEY: ${{ secrets.EVIDRA_SIGNING_KEY }}
  run: |
    evidra run \
      --tool terraform \
      --operation apply \
      --artifact plan.json \
      --environment production \
      -- terraform apply -auto-approve tfplan
```

### GitLab CI

```yaml
deploy:
  variables:
    EVIDRA_METRICS_TRANSPORT: otlp_http
    EVIDRA_METRICS_OTLP_ENDPOINT: $OTLP_ENDPOINT
    EVIDRA_SIGNING_KEY: $EVIDRA_SIGNING_KEY
  script:
    - |
      curl -fsSL https://github.com/samebits/evidra/releases/latest/download/evidra_linux_amd64.tar.gz \
        | tar -xz -C /usr/local/bin evidra
    - evidra run \
        --tool terraform \
        --operation apply \
        --artifact plan.json \
        --environment production \
        -- terraform apply -auto-approve tfplan
```

## What to Monitor

| Dashboard panel | Query basis | Why |
|---|---|---|
| Operations/hour | `signal_count{signal_name="none"}` rate | Throughput baseline |
| Failure rate | `result_class="failure"` / total | Reliability SLI |
| Signal breakdown | `signal_count` by `signal_name` | Root cause categories |
| Score band trend | `signal_count` by `score_band` | Reliability trajectory |
| Duration p50/p95 | `duration_ms` quantiles | Performance regression |
| Retry storms | `signal_count{signal_name="retry_loop"}` | Automation instability |
| Drift frequency | `signal_count{signal_name="artifact_drift"}` | Config consistency |

## Alerting Recommendations

| Alert | Condition | Severity |
|---|---|---|
| High failure rate | `failure_rate > 0.2` over 1h | Warning |
| Retry storm | `retry_loop count > 5` in 1h window | Critical |
| Score degradation | `score_band="poor"` count increasing | Warning |
| Thrashing detected | `thrashing signal > 0` | Critical |
| Metrics export down | No data points for 30m | Info |
