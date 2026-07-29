package api

import "context"

type contextKey string

const wsKey contextKey = "workspace"

// contextWithWorkspace returns a new context with the workspace handle.
func contextWithWorkspace(ctx context.Context, wsHandle string) context.Context {
	return context.WithValue(ctx, wsKey, wsHandle)
}

// workspaceFromContext extracts the workspace handle from the context.
func workspaceFromContext(ctx context.Context) string {
	v, _ := ctx.Value(wsKey).(string)
	return v
}
