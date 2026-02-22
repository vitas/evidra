package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"evidra/internal/demo/gitops"
)

const (
	giteaNamespace = "evidra-demo-gitops"
	giteaService   = "gitea-http"
	argocdNS       = "argocd"
	appNamespace   = "demo"
	appName        = "guestbook-demo"
	evidraBaseURL  = "http://localhost:8080"
	giteaLocalURL  = "http://127.0.0.1:13000"
)

type applicationStatus struct {
	Status struct {
		Sync struct {
			Status   string `json:"status"`
			Revision string `json:"revision"`
		} `json:"sync"`
		Health struct {
			Status string `json:"status"`
		} `json:"health"`
		OperationState struct {
			Phase string `json:"phase"`
		} `json:"operationState"`
	} `json:"status"`
}

type subjectsResponse struct {
	Items []struct {
		Subject   string `json:"subject"`
		Namespace string `json:"namespace"`
		Cluster   string `json:"cluster"`
	} `json:"items"`
}

type changesResponse struct {
	Items []struct {
		ID               string `json:"id"`
		ResultStatus     string `json:"result_status"`
		HealthStatus     string `json:"health_status"`
		ExternalChangeID string `json:"external_change_id"`
		TicketID         string `json:"ticket_id"`
	} `json:"items"`
}

type giteaAuth struct {
	Username string
	Password string
}

func TestKindDemoCases(t *testing.T) {
	if os.Getenv("EVIDRA_KIND_CASES") != "1" {
		t.Skip("set EVIDRA_KIND_CASES=1 to run kind demo e2e cases")
	}

	requireCmd(t, "kubectl")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	assertSandboxReady(ctx, t)
	stopPF := startPortForward(ctx, t, giteaNamespace, "svc/"+giteaService, "13000:3000")
	defer stopPF()
	waitHTTP(ctx, t, giteaLocalURL+"/")
	auth := loadGiteaAuth(ctx, t)
	repoURL := fmt.Sprintf("http://127.0.0.1:13000/%s/demo-app.git", url.PathEscape(auth.Username))
	client := gitops.Client{RepoURL: repoURL, Username: auth.Username, Password: auth.Password}

	fromTS := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)

	t.Run("Case01_Success", func(t *testing.T) {
		if _, err := client.PushCase(ctx, gitops.CaseA); err != nil {
			t.Fatalf("push case A: %v", err)
		}
		triggerSync(ctx, t)
		waitForAppSuccess(ctx, t)
	})

	t.Run("Case02_Correlated", func(t *testing.T) {
		if _, err := client.PushCase(ctx, gitops.CaseB); err != nil {
			t.Fatalf("push case B: %v", err)
		}
		patchAppAnnotations(ctx, t, map[string]string{
			"evidra.io/change-id": "CHG777000",
			"evidra.io/ticket":    "OPS-900",
		})
		triggerSync(ctx, t)
		waitForAppSuccess(ctx, t)
	})

	t.Run("Case03_Failure", func(t *testing.T) {
		if _, err := client.PushCase(ctx, gitops.CaseC); err != nil {
			t.Fatalf("push case C: %v", err)
		}
		triggerSync(ctx, t)
		waitForAppFailureOrDegraded(ctx, t)
	})

	t.Run("FinalValidation", func(t *testing.T) {
		subject := waitForSubject(ctx, t, appName)
		toTS := time.Now().UTC().Format(time.RFC3339)
		deadline := time.Now().Add(5 * time.Minute)
		var (
			changes       changesResponse
			hasCorrelated bool
			failedID      string
		)
		for time.Now().Before(deadline) {
			changes = fetchChanges(t, subject, fromTS, toTS)
			hasCorrelated = false
			failedID = ""
			for _, item := range changes.Items {
				if item.ExternalChangeID == "CHG777000" && item.TicketID == "OPS-900" {
					hasCorrelated = true
				}
				if item.ResultStatus == "failed" || item.HealthStatus == "degraded" {
					failedID = item.ID
				}
			}
			if len(changes.Items) >= 3 && hasCorrelated && failedID != "" {
				break
			}
			time.Sleep(5 * time.Second)
			toTS = time.Now().UTC().Format(time.RFC3339)
		}
		if len(changes.Items) < 3 {
			t.Fatalf("expected at least 3 changes")
		}
		if !hasCorrelated {
			t.Fatalf("expected correlated change with CHG777000 and OPS-900")
		}
		if failedID == "" {
			t.Fatalf("expected at least one failed/degraded change")
		}

		permalink := fmt.Sprintf(
			"%s/ui/explorer/change/%s?subject=%s&from=%s&to=%s",
			evidraBaseURL,
			failedID,
			url.QueryEscape(subject),
			url.QueryEscape(fromTS),
			url.QueryEscape(toTS),
		)
		t.Logf("Argo CD URL: https://localhost:8081")
		t.Logf("Evidra URL: %s/ui/", evidraBaseURL)
		t.Logf("Failed change permalink: %s", permalink)
		t.Logf("Demo scenario completed successfully. %d Changes detected.", len(changes.Items))
	})
}

func assertSandboxReady(ctx context.Context, t *testing.T) {
	t.Helper()
	assertEvidraHealthy(ctx, t)
	runCmd(ctx, t, "kubectl", "get", "statefulset", "gitea", "-n", giteaNamespace)
	runCmd(ctx, t, "kubectl", "-n", giteaNamespace, "rollout", "status", "statefulset/gitea", "--timeout=300s")
	runCmd(ctx, t, "kubectl", "get", "namespace", appNamespace)
	runCmd(ctx, t, "kubectl", "-n", argocdNS, "get", "application", appName)
}

func loadGiteaAuth(ctx context.Context, t *testing.T) giteaAuth {
	t.Helper()
	usernameB64 := strings.TrimSpace(runCmd(ctx, t, "kubectl", "-n", giteaNamespace, "get", "secret", "gitea-admin", "-o", "jsonpath={.data.GITEA_ADMIN_USERNAME}"))
	passwordB64 := strings.TrimSpace(runCmd(ctx, t, "kubectl", "-n", giteaNamespace, "get", "secret", "gitea-admin", "-o", "jsonpath={.data.GITEA_ADMIN_PASSWORD}"))
	username, err := decodeB64(usernameB64)
	if err != nil {
		t.Fatalf("decode username: %v", err)
	}
	password, err := decodeB64(passwordB64)
	if err != nil {
		t.Fatalf("decode password: %v", err)
	}
	return giteaAuth{Username: username, Password: password}
}

func patchAppAnnotations(ctx context.Context, t *testing.T, annotations map[string]string) {
	t.Helper()
	patch := map[string]any{"metadata": map[string]any{"annotations": annotations}}
	body, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}
	runCmd(ctx, t, "kubectl", "-n", argocdNS, "patch", "application", appName, "--type", "merge", "-p", string(body))
}

func triggerSync(ctx context.Context, t *testing.T) {
	t.Helper()
	runCmd(ctx, t, "kubectl", "-n", argocdNS, "patch", "application", appName, "--type", "merge", "-p", `{"operation":{"sync":{"prune":false}}}`)
}

func waitForAppSuccess(ctx context.Context, t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		app := getApplication(ctx, t)
		if app.Status.Health.Status == "Healthy" &&
			app.Status.OperationState.Phase == "Succeeded" {
			return
		}
		time.Sleep(4 * time.Second)
	}
	t.Fatalf("timeout waiting for successful sync")
}

func waitForAppFailureOrDegraded(ctx context.Context, t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		app := getApplication(ctx, t)
		if app.Status.OperationState.Phase == "Failed" || app.Status.OperationState.Phase == "Error" || app.Status.Health.Status == "Degraded" {
			return
		}
		time.Sleep(4 * time.Second)
	}
	t.Fatalf("timeout waiting for app failure/degraded")
}

func getApplication(ctx context.Context, t *testing.T) applicationStatus {
	t.Helper()
	out := runCmd(ctx, t, "kubectl", "-n", argocdNS, "get", "application", appName, "-o", "json")
	var app applicationStatus
	if err := json.Unmarshal([]byte(out), &app); err != nil {
		t.Fatalf("decode application json: %v", err)
	}
	return app
}

func waitForSubject(ctx context.Context, t *testing.T, app string) string {
	t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := http.Get(evidraBaseURL + "/v1/subjects")
		if err == nil && resp.StatusCode == http.StatusOK {
			var body subjectsResponse
			if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
				for _, item := range body.Items {
					if item.Subject == app {
						_ = resp.Body.Close()
						return item.Subject + ":" + item.Namespace + ":" + item.Cluster
					}
				}
			}
			_ = resp.Body.Close()
		}
		time.Sleep(4 * time.Second)
	}
	t.Fatalf("subject not found in Evidra for app %s", app)
	return ""
}

func waitForChanges(ctx context.Context, t *testing.T, subject, fromTS, toTS string, min int) changesResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		body := fetchChanges(t, subject, fromTS, toTS)
		if len(body.Items) >= min {
			return body
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("expected at least %d changes", min)
	return changesResponse{}
}

func fetchChanges(t *testing.T, subject, fromTS, toTS string) changesResponse {
	t.Helper()
	u := evidraBaseURL + "/v1/changes?subject=" + url.QueryEscape(subject) + "&from=" + url.QueryEscape(fromTS) + "&to=" + url.QueryEscape(toTS)
	resp, err := http.Get(u)
	if err != nil {
		return changesResponse{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return changesResponse{}
	}
	var body changesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return changesResponse{}
	}
	return body
}

func assertEvidraHealthy(ctx context.Context, t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(evidraBaseURL + "/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("Evidra is not healthy")
}

func startPortForward(ctx context.Context, t *testing.T, namespace, resource, mapping string) func() {
	t.Helper()
	cmd := exec.CommandContext(ctx, "kubectl", "-n", namespace, "port-forward", resource, mapping)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start port-forward: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "Forwarding from") {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	return func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

func waitHTTP(ctx context.Context, t *testing.T, endpoint string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode < 500 {
			_ = resp.Body.Close()
			return
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("endpoint not reachable: %s", endpoint)
}

func requireCmd(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Fatalf("required command not found: %s", name)
	}
}

func runCmd(ctx context.Context, t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %s %s\n%s", name, strings.Join(args, " "), string(out))
	}
	return string(out)
}

func decodeB64(raw string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
