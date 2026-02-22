.PHONY: evidra-demo evidra-demo-test evidra-demo-all evidra-demo-clean evidra-ui-refresh evidra-ui-sync evidra-ui-e2e-up evidra-ui-e2e evidra-ui-e2e-headed evidra-ui-e2e-down trial-apply trial-smoke trial-port-forward boundary-check

evidra-demo:
	bash scripts/demo-sandbox-up.sh

evidra-demo-test:
	bash scripts/argocd-port-forward.sh start
	EVIDRA_KIND_CASES=1 go test -v ./test/e2e -run TestKindDemoCases

evidra-demo-all: evidra-demo evidra-demo-test

evidra-demo-clean:
	bash scripts/demo-sandbox-down.sh

evidra-ui-sync:
	npm --prefix ui run build
	rsync -a --delete ui/dist/ internal/api/ui/

evidra-ui-refresh: evidra-ui-sync
	bash scripts/install-argocd-extension.sh

evidra-ui-e2e-up:
	make evidra-demo
	bash scripts/argocd-port-forward.sh start

evidra-ui-e2e:
	npm --prefix ui run e2e

evidra-ui-e2e-headed:
	npm --prefix ui run e2e:headed

evidra-ui-e2e-down:
	bash scripts/argocd-port-forward.sh stop
	make evidra-demo-clean

trial-apply:
	bash scripts/ensure-k8s-secrets-env.sh --overlay trial --demo
	kubectl apply -k deploy/k8s/overlays/trial
	EVIDRA_SECRET_NAME=evidra-secrets-trial bash scripts/ensure-k8s-secrets-env.sh --overlay trial --validate-secret

trial-port-forward:
	kubectl -n evidra port-forward svc/evidra-trial 18080:80

trial-smoke:
	bash scripts/smoke-k8s-trial.sh

boundary-check:
	bash scripts/check-internal-boundaries.sh
