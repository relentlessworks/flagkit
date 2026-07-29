package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/relentlessworks/flagkit/internal/model"
)

// wantsJSON checks if the client wants JSON output.
func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return true
	}
	return r.URL.Query().Get("format") == "json"
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeText writes a plain text response.
func writeText(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintln(w, text)
}

// writeError writes an error response in the appropriate format.
func writeError(w http.ResponseWriter, r *http.Request, status int, msg, hint string) {
	if wantsJSON(r) {
		writeJSON(w, status, map[string]string{
			"error": msg,
			"hint":  hint,
		})
		return
	}
	writeText(w, status, fmt.Sprintf("error: %s | hint: %s", msg, hint))
}

// writeOK writes a success response.
func writeOK(w http.ResponseWriter, r *http.Request, text string) {
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": text})
		return
	}
	writeText(w, http.StatusOK, text)
}

// formatFlag formats a flag as a single grepable line.
func formatFlag(f *model.Flag) string {
	parts := []string{
		fmt.Sprintf("handle=%s", f.Handle),
		fmt.Sprintf("name=%s", f.Name),
		fmt.Sprintf("type=%s", f.Type),
		fmt.Sprintf("enabled=%v", f.Enabled),
	}
	if f.Description != "" {
		parts = append(parts, fmt.Sprintf("desc=%s", f.Description))
	}
	if f.Type == model.FlagTypePercentage {
		parts = append(parts, fmt.Sprintf("percentage=%d", f.Percentage))
	}
	if f.Type == model.FlagTypeVariant && len(f.Variants) > 0 {
		parts = append(parts, fmt.Sprintf("variants=%s", strings.Join(f.Variants, ",")))
	}
	if f.DefaultVar != "" {
		parts = append(parts, fmt.Sprintf("default_variant=%s", f.DefaultVar))
	}
	if len(f.Tags) > 0 {
		parts = append(parts, fmt.Sprintf("tags=%s", strings.Join(f.Tags, ",")))
	}
	return strings.Join(parts, " ")
}

// formatEvaluateResult formats an evaluation result as a single line.
func formatEvaluateResult(r model.EvaluateResult) string {
	parts := []string{
		fmt.Sprintf("handle=%s", r.Handle),
		fmt.Sprintf("enabled=%v", r.Enabled),
	}
	if r.Variant != "" {
		parts = append(parts, fmt.Sprintf("variant=%s", r.Variant))
	}
	parts = append(parts, fmt.Sprintf("reason=%s", r.Reason))
	return strings.Join(parts, " ")
}

// formatAudit formats an audit entry as a single line.
func formatAudit(e model.AuditEntry) string {
	parts := []string{
		fmt.Sprintf("id=%d", e.ID),
		fmt.Sprintf("action=%s", e.Action),
	}
	if e.Handle != "" {
		parts = append(parts, fmt.Sprintf("handle=%s", e.Handle))
	}
	if e.Detail != "" {
		parts = append(parts, fmt.Sprintf("detail=%s", e.Detail))
	}
	if e.Email != "" {
		parts = append(parts, fmt.Sprintf("email=%s", e.Email))
	}
	parts = append(parts, fmt.Sprintf("time=%s", e.Timestamp.Format("2006-01-02T15:04:05Z")))
	return strings.Join(parts, " ")
}
