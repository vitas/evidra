# Argo Annotation Integration v1

Use Argo Application annotations as the primary external traceability mechanism.

## Supported keys
- `evidra.io/change-id`
- `evidra.io/ticket`
- `evidra.io/approvals-ref`
- `evidra.io/approvals-json` (optional)

## Why this approach
- Keeps integration lightweight and Argo-first.
- Avoids mandatory direct API integrations with external systems.
- Preserves deterministic evidence at deployment time.

## ApplicationSet propagation
For generated Applications, set the same annotations in:
- `ApplicationSet.spec.template.metadata.annotations`

## Minimal examples
Helm values/template, Kustomize, and ApplicationSet snippets are in:
- `docs/setup/adoption-guide.md`
