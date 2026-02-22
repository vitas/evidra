package argo

import (
	"fmt"
	"strings"
)

type BackendOptions struct {
	Backend string
	URL     string
	Token   string
}

func NewFetchFunc(opts BackendOptions) (FetchFunc, error) {
	backend := strings.ToLower(strings.TrimSpace(opts.Backend))
	if backend == "" || backend == "argocd-client" {
		return NewArgoCDClientFetcher(opts)
	}
	return nil, fmt.Errorf("unsupported argo backend: %s", opts.Backend)
}
