package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

type Client struct {
	RepoURL  string
	Username string
	Password string
}

type CommitCase string

const (
	CaseA CommitCase = "A"
	CaseB CommitCase = "B"
	CaseC CommitCase = "C"
)

func (c Client) PushCase(ctx context.Context, cc CommitCase) (string, error) {
	if strings.TrimSpace(c.RepoURL) == "" {
		return "", fmt.Errorf("repo url is required")
	}
	auth := &githttp.BasicAuth{Username: c.Username, Password: c.Password}
	tmpDir, err := os.MkdirTemp("", "evidra-demo-gitops-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	repo, wt, err := cloneOrInit(ctx, tmpDir, c.RepoURL, auth)
	if err != nil {
		return "", err
	}

	if err := writeCaseFiles(tmpDir, cc); err != nil {
		return "", err
	}
	if err := wt.AddGlob("."); err != nil {
		return "", err
	}

	hash, err := wt.Commit("demo commit "+string(cc), &git.CommitOptions{Author: &object.Signature{
		Name:  "evidra-demo",
		Email: "demo@evidra.local",
		When:  time.Now().UTC(),
	}})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "empty commit") {
			return "", nil
		}
		return "", err
	}

	sourceRef := "HEAD"
	if headRef, err := repo.Head(); err == nil && headRef.Name().String() != "" {
		sourceRef = headRef.Name().String()
	}

	if err := repo.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs:   []config.RefSpec{config.RefSpec(sourceRef + ":refs/heads/main")},
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return "", err
	}

	return hash.String(), nil
}

func cloneOrInit(ctx context.Context, dir, repoURL string, auth *githttp.BasicAuth) (*git.Repository, *git.Worktree, error) {
	repo, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:      repoURL,
		Auth:     auth,
		Progress: os.Stdout,
	})
	if err != nil {
		repo, err = git.PlainInit(dir, false)
		if err != nil {
			return nil, nil, err
		}
		if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{repoURL}}); err != nil {
			return nil, nil, err
		}
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, nil, err
	}
	return repo, wt, nil
}

func writeCaseFiles(root string, cc CommitCase) error {
	appDir := filepath.Join(root, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return err
	}

	kustomization := "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - deployment.yaml\n  - service.yaml\n  - configmap.yaml\n"
	if cc == CaseC {
		kustomization += "  - missing.yaml\n"
	}
	version := "v1"
	if cc == CaseB {
		version = "v2"
	}
	if cc == CaseC {
		version = "v3"
	}

	files := map[string]string{
		filepath.Join(appDir, "kustomization.yaml"): kustomization,
		filepath.Join(appDir, "configmap.yaml"): `apiVersion: v1
kind: ConfigMap
metadata:
  name: guestbook-demo-config
data:
  VERSION: ` + version + `
`,
		filepath.Join(appDir, "service.yaml"): `apiVersion: v1
kind: Service
metadata:
  name: guestbook-demo
spec:
  selector:
    app: guestbook-demo
  ports:
    - port: 80
      targetPort: 8080
`,
		filepath.Join(appDir, "deployment.yaml"): `apiVersion: apps/v1
kind: Deployment
metadata:
  name: guestbook-demo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: guestbook-demo
  template:
    metadata:
      labels:
        app: guestbook-demo
    spec:
      containers:
        - name: guestbook-demo
          image: registry.k8s.io/e2e-test-images/agnhost:2.45
          args: ["netexec", "--http-port=8080"]
          ports:
            - containerPort: 8080
          envFrom:
            - configMapRef:
                name: guestbook-demo-config
`,
	}

	for p, content := range files {
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
