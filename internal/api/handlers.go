package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/relentlessworks/flagkit/internal/auth"
	"github.com/relentlessworks/flagkit/internal/model"
	"github.com/relentlessworks/flagkit/internal/store"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	store *store.Store
	auth  *auth.Auth
}

// New creates a new API handler.
func New(s *store.Store, a *auth.Auth) *Handler {
	return &Handler{store: s, auth: a}
}

// Routes returns the HTTP mux with all routes registered.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("/help", h.handleHelp)
	mux.HandleFunc("/.well-known/agent.md", h.handleHelp)
	mux.HandleFunc("/auth/request", h.handleAuthRequest)
	mux.HandleFunc("/auth/verify", h.handleAuthVerify)

	// Authenticated endpoints
	mux.HandleFunc("/flags", h.authMiddleware(h.handleFlags))
	mux.HandleFunc("/flags/", h.authMiddleware(h.handleFlagByHandle))
	mux.HandleFunc("/audit", h.authMiddleware(h.handleAudit))

	// MCP endpoint
	mux.HandleFunc("/mcp", h.handleMCP)

	return mux
}

// --- Help ---

func (h *Handler) handleHelp(w http.ResponseWriter, r *http.Request) {
	manual := `flagkit — agentic-first feature flag management service

AUTH:
  1. POST /auth/request  body: email=user@example.com
     → Sends OTP code (in dev mode, returned in response)
  2. POST /auth/verify   body: email=user@example.com code=123456
     → Returns: token=fk_xxx ws_handle=ws_abc12
  3. Use token in all subsequent requests:
     Authorization: Bearer fk_xxx

FLAGS:
  Create a flag:
    POST /flags  body: name=my_flag type=boolean
    Types: boolean, percentage, variant
    Optional: desc=description, enabled=true, percentage=50, variants=red,green,blue, default_variant=red, tags=prod,beta
    → handle=flag_k7m2q name=my_flag type=boolean enabled=true

  List flags:
    GET /flags
    → One line per flag: handle=flag_xxx name=xxx type=xxx enabled=xxx

  Get a flag:
    GET /flags/{handle}
    → handle=flag_xxx name=xxx type=xxx enabled=xxx

  Update a flag:
    PATCH /flags/{handle}  body: enabled=false (or name, desc, percentage, variants, tags)
    → handle=flag_xxx name=xxx type=xxx enabled=false

  Delete a flag:
    DELETE /flags/{handle}
    → ok: flag deleted

  Evaluate a flag:
    POST /flags/{handle}/evaluate  body: context=user123
    → handle=flag_xxx enabled=true reason=boolean flag is enabled
    For variant flags: handle=flag_xxx enabled=true variant=red reason=variant selected: red

AUDIT:
  GET /audit?limit=20
  → One line per entry: id=1 action=flag_created handle=flag_xxx time=2026-01-01T00:00:00Z

FORMATS:
  Default: plain text (one record per line, space-separated key=value)
  JSON:    add Accept: application/json header or ?format=json query param

ERRORS:
  All 4xx responses include a hint:
  error: missing auth token | hint: call POST /auth/request with email to get an OTP

MCP:
  GET /mcp — Model Context Protocol endpoint for chat client integrations
`
	writeText(w, http.StatusOK, manual)
}

// --- Auth ---

func (h *Handler) handleAuthRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
		return
	}

	email := r.FormValue("email")
	if email == "" {
		// Try JSON body
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			email = body["email"]
		}
	}
	if email == "" {
		writeError(w, r, http.StatusBadRequest, "email is required", "include email in the request body: email=user@example.com")
		return
	}

	code, err := h.auth.RequestOTP(email)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to generate OTP", "try again")
		return
	}

	// In dev mode, return the code directly (no SMTP configured)
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]string{
			"ok":     "OTP sent",
			code_key: code,
		})
		return
	}
	// Dev mode: include code in response for testing
	writeText(w, http.StatusOK, fmt.Sprintf("ok: OTP sent to %s | code: %s (dev mode — no SMTP configured)", email, code))
}

const code_key = "code"

func (h *Handler) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST")
		return
	}

	email := r.FormValue("email")
	code := r.FormValue("code")
	if email == "" || code == "" {
		// Try JSON body
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if email == "" {
				email = body["email"]
			}
			if code == "" {
				code = body["code"]
			}
		}
	}
	if email == "" || code == "" {
		writeError(w, r, http.StatusBadRequest, "email and code are required", "include both: email=user@example.com code=123456")
		return
	}

	token, err := h.auth.VerifyOTP(email, code)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, err.Error(), "request a new OTP via POST /auth/request with your email")
		return
	}

	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]string{
			"token":      token.Token,
			"ws_handle":  token.Workspace,
			"email":      token.Email,
		})
		return
	}
	writeText(w, http.StatusOK, fmt.Sprintf("token=%s ws_handle=%s email=%s", token.Token, token.Workspace, token.Email))
}

// --- Flags ---

func (h *Handler) handleFlags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createFlag(w, r)
	case http.MethodGet:
		h.listFlags(w, r)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST to create or GET to list flags")
	}
}

func (h *Handler) createFlag(w http.ResponseWriter, r *http.Request) {
	wsHandle := workspaceFromContext(r.Context())

	var input struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Type        string   `json:"type"`
		Enabled     *bool    `json:"enabled"`
		Percentage  int      `json:"percentage"`
		Variants    []string `json:"variants"`
		DefaultVar  string   `json:"default_variant"`
		Tags        []string `json:"tags"`
	}

	// Parse body (form or JSON)
	if isJSONContent(r) {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid JSON body", "send a valid JSON object with name and type fields")
			return
		}
	} else {
		input.Name = r.FormValue("name")
		input.Description = r.FormValue("desc")
		input.Type = r.FormValue("type")
		input.DefaultVar = r.FormValue("default_variant")
		if v := r.FormValue("percentage"); v != "" {
			fmt.Sscanf(v, "%d", &input.Percentage)
		}
		if v := r.FormValue("variants"); v != "" {
			input.Variants = strings.Split(v, ",")
		}
		if v := r.FormValue("tags"); v != "" {
			input.Tags = strings.Split(v, ",")
		}
		if v := r.FormValue("enabled"); v == "true" || v == "1" {
			t := true
			input.Enabled = &t
		}
	}

	if input.Name == "" {
		writeError(w, r, http.StatusBadRequest, "name is required", "include name in the request body: name=my_flag")
		return
	}
	if input.Type == "" {
		input.Type = string(model.FlagTypeBoolean)
	}

	flag := &model.Flag{
		Handle:      model.GenerateHandle(),
		Name:        input.Name,
		Description: input.Description,
		Type:        model.FlagType(input.Type),
		Enabled:     input.Enabled != nil && *input.Enabled,
		Percentage:  input.Percentage,
		Variants:    input.Variants,
		DefaultVar:  input.DefaultVar,
		Tags:        input.Tags,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := flag.Validate(); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error(), "check the flag type and required fields for that type")
		return
	}

	if err := h.store.CreateFlag(wsHandle, flag); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to create flag", "try again with a different name")
		return
	}

	_ = h.store.AddAudit(wsHandle, model.AuditEntry{
		Action: "flag_created",
		Handle: flag.Handle,
		Detail: fmt.Sprintf("name=%s type=%s", flag.Name, flag.Type),
	})

	if wantsJSON(r) {
		writeJSON(w, http.StatusCreated, flag)
		return
	}
	writeText(w, http.StatusCreated, formatFlag(flag))
}

func (h *Handler) listFlags(w http.ResponseWriter, r *http.Request) {
	wsHandle := workspaceFromContext(r.Context())

	flags, err := h.store.ListFlags(wsHandle)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to list flags", "try again")
		return
	}

	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, flags)
		return
	}

	if len(flags) == 0 {
		writeText(w, http.StatusOK, "ok: no flags found")
		return
	}

	var lines []string
	for _, f := range flags {
		lines = append(lines, formatFlag(f))
	}
	writeText(w, http.StatusOK, strings.Join(lines, "\n"))
}

func (h *Handler) handleFlagByHandle(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/flags/")
	parts := strings.SplitN(path, "/", 2)
	handle := parts[0]

	if handle == "" {
		writeError(w, r, http.StatusBadRequest, "flag handle is required", "use GET /flags/{handle} to get a specific flag")
		return
	}

	// Check for /evaluate sub-path
	if len(parts) == 2 && parts[1] == "evaluate" {
		if r.Method != http.MethodPost {
			writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST to evaluate a flag")
			return
		}
		h.evaluateFlag(w, r, handle)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getFlag(w, r, handle)
	case http.MethodPatch:
		h.updateFlag(w, r, handle)
	case http.MethodDelete:
		h.deleteFlag(w, r, handle)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET, PATCH, or DELETE")
	}
}

func (h *Handler) getFlag(w http.ResponseWriter, r *http.Request, handle string) {
	wsHandle := workspaceFromContext(r.Context())

	flag, err := h.store.GetFlag(wsHandle, handle)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "flag not found", "call GET /flags to list all flags")
		return
	}

	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, flag)
		return
	}
	writeText(w, http.StatusOK, formatFlag(flag))
}

func (h *Handler) updateFlag(w http.ResponseWriter, r *http.Request, handle string) {
	wsHandle := workspaceFromContext(r.Context())

	flag, err := h.store.GetFlag(wsHandle, handle)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "flag not found", "call GET /flags to list all flags")
		return
	}

	var input struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
		Enabled     *bool    `json:"enabled"`
		Percentage  *int     `json:"percentage"`
		Variants    []string `json:"variants"`
		DefaultVar  *string  `json:"default_variant"`
		Tags        []string `json:"tags"`
	}

	if isJSONContent(r) {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, r, http.StatusBadRequest, "invalid JSON body", "send a valid JSON object with fields to update")
			return
		}
	} else {
		if v := r.FormValue("name"); v != "" {
			input.Name = &v
		}
		if v := r.FormValue("desc"); v != "" {
			input.Description = &v
		}
		if v := r.FormValue("enabled"); v != "" {
			b := v == "true" || v == "1"
			input.Enabled = &b
		}
		if v := r.FormValue("percentage"); v != "" {
			var p int
			fmt.Sscanf(v, "%d", &p)
			input.Percentage = &p
		}
		if v := r.FormValue("variants"); v != "" {
			input.Variants = strings.Split(v, ",")
		}
		if v := r.FormValue("default_variant"); v != "" {
			input.DefaultVar = &v
		}
		if v := r.FormValue("tags"); v != "" {
			input.Tags = strings.Split(v, ",")
		}
	}

	// Apply updates
	if input.Name != nil {
		flag.Name = *input.Name
	}
	if input.Description != nil {
		flag.Description = *input.Description
	}
	if input.Enabled != nil {
		flag.Enabled = *input.Enabled
	}
	if input.Percentage != nil {
		flag.Percentage = *input.Percentage
	}
	if input.Variants != nil {
		flag.Variants = input.Variants
	}
	if input.DefaultVar != nil {
		flag.DefaultVar = *input.DefaultVar
	}
	if input.Tags != nil {
		flag.Tags = input.Tags
	}

	if err := flag.Validate(); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error(), "check the flag type and required fields for that type")
		return
	}

	if err := h.store.UpdateFlag(wsHandle, flag); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to update flag", "try again")
		return
	}

	_ = h.store.AddAudit(wsHandle, model.AuditEntry{
		Action: "flag_updated",
		Handle: flag.Handle,
		Detail: fmt.Sprintf("enabled=%v", flag.Enabled),
	})

	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, flag)
		return
	}
	writeText(w, http.StatusOK, formatFlag(flag))
}

func (h *Handler) deleteFlag(w http.ResponseWriter, r *http.Request, handle string) {
	wsHandle := workspaceFromContext(r.Context())

	_, err := h.store.GetFlag(wsHandle, handle)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "flag not found", "call GET /flags to list all flags")
		return
	}

	if err := h.store.DeleteFlag(wsHandle, handle); err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to delete flag", "try again")
		return
	}

	_ = h.store.AddAudit(wsHandle, model.AuditEntry{
		Action: "flag_deleted",
		Handle: handle,
	})

	writeOK(w, r, "flag deleted")
}

func (h *Handler) evaluateFlag(w http.ResponseWriter, r *http.Request, handle string) {
	wsHandle := workspaceFromContext(r.Context())

	flag, err := h.store.GetFlag(wsHandle, handle)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "flag not found", "call GET /flags to list all flags")
		return
	}

	contextKey := r.FormValue("context")
	if contextKey == "" {
		if isJSONContent(r) {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
				contextKey = body["context"]
			}
		}
	}

	result := flag.Evaluate(contextKey)

	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeText(w, http.StatusOK, formatEvaluateResult(result))
}

// --- Audit ---

func (h *Handler) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use GET to list audit entries")
		return
	}

	wsHandle := workspaceFromContext(r.Context())

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}

	entries, err := h.store.ListAudit(wsHandle, limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "failed to list audit entries", "try again")
		return
	}

	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, entries)
		return
	}

	if len(entries) == 0 {
		writeText(w, http.StatusOK, "ok: no audit entries found")
		return
	}

	var lines []string
	for _, e := range entries {
		lines = append(lines, formatAudit(e))
	}
	writeText(w, http.StatusOK, strings.Join(lines, "\n"))
}

// --- Helpers ---

func isJSONContent(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.Contains(ct, "application/json")
}
