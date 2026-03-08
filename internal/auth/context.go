// Package auth provides authentication middleware and context helpers.
package auth

import "context"

type ctxKey struct{}

// WithTenantID stores a tenant ID in the context.
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, tenantID)
}

// TenantID retrieves the tenant ID from the context.
// Returns empty string if not set.
func TenantID(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}
