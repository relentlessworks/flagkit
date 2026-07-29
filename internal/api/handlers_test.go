package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/relentlessworks/flagkit/internal/auth"
	"github.com/relentlessworks/flagkit/internal/model"
	"github.com/relentlessworks/flagkit/internal/store"
)

// testHelper creates a store, auth, and handler for testing.
func testHelper(t *testing.T) (*Handler, string) {
	t.Helper()
	tmpFile := fmt.Sprintf("/tmp/flagkit_test_%d.json", time.Now().UnixNano())
	st, err := store.New(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	authSvc := auth.New(st)
	h := New(st, authSvc)

	// Create a workspace and token for testing
	wsHandle := model.GenerateWorkspaceHandle()
	_ = st.CreateWorkspace(&model.Workspace{
		Handle:    wsHandle,
		Name:      "test@example.com",
		Plan:      "free",
		CreatedAt: time.Now(),
	})
	token := model.GenerateToken()
	_ = st.SaveToken(&model.Token{
		Token:     token,
		Workspace: wsHandle,
		Email:     "test@example.com",
		CreatedAt: time.Now(),
	})

	return h, token
}

// authedRequest creates a request with the auth token set.
func authedRequest(method, url, token string, body string) *http.Request {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, url, bodyReader)
	if strings.Contains(body, "{") {
		req.Header.Set("Content-Type", "application/json")
	} else if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// callAuthed wraps a handler with auth middleware and calls it.
func callAuthed(h *Handler, handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.authMiddleware(handler)(w, req)
	return w
}

func TestHelp(t *testing.T) {
	h, _ := testHelper(t)
	req := httptest.NewRequest("GET", "/help", nil)
	w := httptest.NewRecorder()
	h.handleHelp(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "flagkit") {
		t.Error("help should mention flagkit")
	}
	if !strings.Contains(body, "AUTH") {
		t.Error("help should include AUTH section")
	}
	if !strings.Contains(body, "FLAGS") {
		t.Error("help should include FLAGS section")
	}
}

func TestAuthRequestAndVerify(t *testing.T) {
	h, _ := testHelper(t)

	// Request OTP
	body := strings.NewReader("email=test@example.com")
	req := httptest.NewRequest("POST", "/auth/request", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleAuthRequest(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Extract OTP code from response
	respBody := w.Body.String()
	idx := strings.Index(respBody, "code: ")
	if idx < 0 {
		t.Fatal("response should contain code in dev mode")
	}
	// Code is 6 digits after "code: "
	codeStart := idx + 6
	code := respBody[codeStart : codeStart+6]
	if len(code) != 6 {
		t.Fatalf("expected 6-digit code, got: %s", code)
	}

	// Verify OTP
	body2 := strings.NewReader(fmt.Sprintf("email=test@example.com&code=%s", code))
	req2 := httptest.NewRequest("POST", "/auth/verify", body2)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	h.handleAuthVerify(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "token=") {
		t.Error("verify should return a token")
	}
}

func TestAuthRequestMissingEmail(t *testing.T) {
	h, _ := testHelper(t)
	req := httptest.NewRequest("POST", "/auth/request", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleAuthRequest(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAuthVerifyInvalidCode(t *testing.T) {
	h, _ := testHelper(t)

	// Request OTP first
	body := strings.NewReader("email=newuser@example.com")
	req := httptest.NewRequest("POST", "/auth/request", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleAuthRequest(w, req)

	// Verify with wrong code
	body2 := strings.NewReader("email=newuser@example.com&code=000000")
	req2 := httptest.NewRequest("POST", "/auth/verify", body2)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	h.handleAuthVerify(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w2.Code)
	}
}

func TestCreateFlag(t *testing.T) {
	h, token := testHelper(t)

	req := authedRequest("POST", "/flags", token, "name=test_flag&type=boolean&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	if !strings.Contains(resp, "handle=flag_") {
		t.Error("response should contain flag handle")
	}
	if !strings.Contains(resp, "name=test_flag") {
		t.Error("response should contain flag name")
	}
	if !strings.Contains(resp, "type=boolean") {
		t.Error("response should contain flag type")
	}
	if !strings.Contains(resp, "enabled=true") {
		t.Error("response should contain enabled state")
	}
}

func TestCreateFlagJSON(t *testing.T) {
	h, token := testHelper(t)

	flagJSON := `{"name":"json_flag","type":"percentage","percentage":50,"enabled":true}`
	req := authedRequest("POST", "/flags", token, flagJSON)
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	if !strings.Contains(resp, "percentage=50") {
		t.Error("response should contain percentage")
	}
}

func TestCreateFlagVariant(t *testing.T) {
	h, token := testHelper(t)

	req := authedRequest("POST", "/flags", token, "name=ab_test&type=variant&variants=red,green,blue&default_variant=red&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	if !strings.Contains(resp, "variants=red,green,blue") {
		t.Error("response should contain variants")
	}
	if !strings.Contains(resp, "default_variant=red") {
		t.Error("response should contain default variant")
	}
}

func TestCreateFlagMissingName(t *testing.T) {
	h, token := testHelper(t)

	req := authedRequest("POST", "/flags", token, "type=boolean")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateFlagInvalidType(t *testing.T) {
	h, token := testHelper(t)

	req := authedRequest("POST", "/flags", token, "name=bad_flag&type=invalid")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListFlags(t *testing.T) {
	h, token := testHelper(t)

	// Create two flags
	for _, name := range []string{"flag1", "flag2"} {
		req := authedRequest("POST", "/flags", token, fmt.Sprintf("name=%s&type=boolean&enabled=true", name))
		w := callAuthed(h, h.handleFlags, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("failed to create flag: %d: %s", w.Code, w.Body.String())
		}
	}

	// List flags
	req := authedRequest("GET", "/flags", token, "")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	resp := w.Body.String()
	lines := strings.Split(strings.TrimSpace(resp), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 flags, got %d lines: %s", len(lines), resp)
	}
}

func TestListFlagsEmpty(t *testing.T) {
	h, token := testHelper(t)

	req := authedRequest("GET", "/flags", token, "")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "no flags found") {
		t.Error("should indicate no flags found")
	}
}

func extractHandle(t *testing.T, resp string) string {
	t.Helper()
	handleIdx := strings.Index(resp, "handle=") + 7
	handleEnd := strings.Index(resp[handleIdx:], " ")
	if handleEnd < 0 {
		handleEnd = len(resp) - handleIdx
	}
	return resp[handleIdx : handleIdx+handleEnd]
}

func TestGetFlag(t *testing.T) {
	h, token := testHelper(t)

	// Create a flag
	req := authedRequest("POST", "/flags", token, "name=get_test&type=boolean&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create flag: %d: %s", w.Code, w.Body.String())
	}

	handle := extractHandle(t, w.Body.String())

	// Get the flag
	req2 := authedRequest("GET", "/flags/"+handle, token, "")
	w2 := callAuthed(h, h.handleFlagByHandle, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "name=get_test") {
		t.Error("should return the flag")
	}
}

func TestGetFlagNotFound(t *testing.T) {
	h, token := testHelper(t)

	req := authedRequest("GET", "/flags/flag_nonexist", token, "")
	w := callAuthed(h, h.handleFlagByHandle, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateFlag(t *testing.T) {
	h, token := testHelper(t)

	// Create a flag
	req := authedRequest("POST", "/flags", token, "name=update_test&type=boolean&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create flag: %d: %s", w.Code, w.Body.String())
	}

	handle := extractHandle(t, w.Body.String())

	// Update the flag (disable it)
	req2 := authedRequest("PATCH", "/flags/"+handle, token, "enabled=false")
	w2 := callAuthed(h, h.handleFlagByHandle, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "enabled=false") {
		t.Error("flag should be disabled after update")
	}
}

func TestDeleteFlag(t *testing.T) {
	h, token := testHelper(t)

	// Create a flag
	req := authedRequest("POST", "/flags", token, "name=delete_test&type=boolean&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create flag: %d: %s", w.Code, w.Body.String())
	}

	handle := extractHandle(t, w.Body.String())

	// Delete the flag
	req2 := authedRequest("DELETE", "/flags/"+handle, token, "")
	w2 := callAuthed(h, h.handleFlagByHandle, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "flag deleted") {
		t.Error("should confirm deletion")
	}

	// Verify it's gone
	req3 := authedRequest("GET", "/flags/"+handle, token, "")
	w3 := callAuthed(h, h.handleFlagByHandle, req3)
	if w3.Code != http.StatusNotFound {
		t.Errorf("expected 404 after deletion, got %d", w3.Code)
	}
}

func TestEvaluateBooleanFlag(t *testing.T) {
	h, token := testHelper(t)

	// Create a boolean flag (enabled)
	req := authedRequest("POST", "/flags", token, "name=eval_bool&type=boolean&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	handle := extractHandle(t, w.Body.String())

	// Evaluate
	req2 := authedRequest("POST", "/flags/"+handle+"/evaluate", token, "context=user123")
	w2 := callAuthed(h, h.handleFlagByHandle, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "enabled=true") {
		t.Error("boolean flag should evaluate to true when enabled")
	}
}

func TestEvaluateDisabledFlag(t *testing.T) {
	h, token := testHelper(t)

	// Create a disabled boolean flag
	req := authedRequest("POST", "/flags", token, "name=eval_disabled&type=boolean&enabled=false")
	w := callAuthed(h, h.handleFlags, req)
	handle := extractHandle(t, w.Body.String())

	// Evaluate
	req2 := authedRequest("POST", "/flags/"+handle+"/evaluate", token, "context=user123")
	w2 := callAuthed(h, h.handleFlagByHandle, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "enabled=false") {
		t.Error("disabled flag should evaluate to false")
	}
	if !strings.Contains(w2.Body.String(), "flag is disabled") {
		t.Error("reason should indicate flag is disabled")
	}
}

func TestEvaluateVariantFlag(t *testing.T) {
	h, token := testHelper(t)

	// Create a variant flag
	req := authedRequest("POST", "/flags", token, "name=eval_variant&type=variant&variants=red,green,blue&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	handle := extractHandle(t, w.Body.String())

	// Evaluate
	req2 := authedRequest("POST", "/flags/"+handle+"/evaluate", token, "context=user123")
	w2 := callAuthed(h, h.handleFlagByHandle, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	resp2 := w2.Body.String()
	if !strings.Contains(resp2, "variant=") {
		t.Error("variant flag should return a variant")
	}
	// Should be one of red, green, or blue
	found := false
	for _, v := range []string{"red", "green", "blue"} {
		if strings.Contains(resp2, "variant="+v) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("variant should be red, green, or blue: %s", resp2)
	}
}

func TestEvaluatePercentageFlag(t *testing.T) {
	h, token := testHelper(t)

	// Create a 100% percentage flag
	req := authedRequest("POST", "/flags", token, "name=eval_pct&type=percentage&percentage=100&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	handle := extractHandle(t, w.Body.String())

	// Evaluate
	req2 := authedRequest("POST", "/flags/"+handle+"/evaluate", token, "context=user123")
	w2 := callAuthed(h, h.handleFlagByHandle, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "enabled=true") {
		t.Error("100% flag should evaluate to true")
	}
}

func TestJSONResponse(t *testing.T) {
	h, token := testHelper(t)

	// Create a flag with JSON request
	flagJSON := `{"name":"json_test","type":"boolean","enabled":true}`
	req := httptest.NewRequest("POST", "/flags", strings.NewReader(flagJSON))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content type, got %s", ct)
	}
	var flag map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&flag); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if flag["name"] != "json_test" {
		t.Errorf("expected name=json_test, got %v", flag["name"])
	}
}

func TestMissingAuth(t *testing.T) {
	h, _ := testHelper(t)

	req := httptest.NewRequest("GET", "/flags", nil)
	w := httptest.NewRecorder()
	h.authMiddleware(h.handleFlags)(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "hint:") {
		t.Error("error should include a hint")
	}
}

func TestInvalidToken(t *testing.T) {
	h, _ := testHelper(t)

	req := httptest.NewRequest("GET", "/flags", nil)
	req.Header.Set("Authorization", "Bearer invalid_token")
	w := httptest.NewRecorder()
	h.authMiddleware(h.handleFlags)(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuditLog(t *testing.T) {
	h, token := testHelper(t)

	// Create a flag (should generate audit entry)
	req := authedRequest("POST", "/flags", token, "name=audit_test&type=boolean&enabled=true")
	w := callAuthed(h, h.handleFlags, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create flag: %d: %s", w.Code, w.Body.String())
	}

	// Check audit log
	req2 := authedRequest("GET", "/audit", token, "")
	w2 := callAuthed(h, h.handleAudit, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "flag_created") {
		t.Error("audit log should contain flag_created entry")
	}
}

func TestMCPInitialize(t *testing.T) {
	h, _ := testHelper(t)

	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleMCP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp MCPResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Error("should return JSON-RPC 2.0")
	}
}

func TestMCPToolsList(t *testing.T) {
	h, _ := testHelper(t)

	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handleMCP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	resp := w.Body.String()
	if !strings.Contains(resp, "list_flags") {
		t.Error("should list list_flags tool")
	}
	if !strings.Contains(resp, "evaluate_flag") {
		t.Error("should list evaluate_flag tool")
	}
}

func TestFlagModelEvaluate(t *testing.T) {
	// Test boolean flag
	bf := model.Flag{Type: model.FlagTypeBoolean, Enabled: true}
	r := bf.Evaluate("user1")
	if !r.Enabled {
		t.Error("boolean flag should be enabled")
	}

	// Test disabled flag
	bf.Enabled = false
	r = bf.Evaluate("user1")
	if r.Enabled {
		t.Error("disabled flag should not be enabled")
	}

	// Test percentage 100%
	pf := model.Flag{Type: model.FlagTypePercentage, Enabled: true, Percentage: 100}
	r = pf.Evaluate("user1")
	if !r.Enabled {
		t.Error("100% flag should be enabled")
	}

	// Test percentage 0%
	pf.Percentage = 0
	r = pf.Evaluate("user1")
	if r.Enabled {
		t.Error("0% flag should not be enabled")
	}

	// Test variant flag
	vf := model.Flag{Type: model.FlagTypeVariant, Enabled: true, Variants: []string{"a", "b", "c"}}
	r = vf.Evaluate("user1")
	if r.Variant == "" {
		t.Error("variant flag should return a variant")
	}
	found := false
	for _, v := range vf.Variants {
		if r.Variant == v {
			found = true
		}
	}
	if !found {
		t.Errorf("variant should be one of %v, got %s", vf.Variants, r.Variant)
	}
}

func TestFlagModelValidate(t *testing.T) {
	// Missing name
	f := model.Flag{Type: model.FlagTypeBoolean}
	if err := f.Validate(); err == nil {
		t.Error("should fail without name")
	}

	// Invalid type
	f = model.Flag{Name: "test", Type: "invalid"}
	if err := f.Validate(); err == nil {
		t.Error("should fail with invalid type")
	}

	// Percentage out of range
	f = model.Flag{Name: "test", Type: model.FlagTypePercentage, Percentage: 150}
	if err := f.Validate(); err == nil {
		t.Error("should fail with percentage > 100")
	}

	// Variant without variants
	f = model.Flag{Name: "test", Type: model.FlagTypeVariant}
	if err := f.Validate(); err == nil {
		t.Error("should fail without variants for variant type")
	}

	// Valid boolean
	f = model.Flag{Name: "test", Type: model.FlagTypeBoolean}
	if err := f.Validate(); err != nil {
		t.Errorf("valid boolean flag should pass: %v", err)
	}

	// Valid percentage
	f = model.Flag{Name: "test", Type: model.FlagTypePercentage, Percentage: 50}
	if err := f.Validate(); err != nil {
		t.Errorf("valid percentage flag should pass: %v", err)
	}

	// Valid variant
	f = model.Flag{Name: "test", Type: model.FlagTypeVariant, Variants: []string{"a", "b"}}
	if err := f.Validate(); err != nil {
		t.Errorf("valid variant flag should pass: %v", err)
	}
}

func TestHandleGeneration(t *testing.T) {
	h1 := model.GenerateHandle()
	h2 := model.GenerateHandle()
	if h1 == h2 {
		t.Error("handles should be unique")
	}
	if !strings.HasPrefix(h1, "flag_") {
		t.Error("handle should start with flag_")
	}
}

func TestRoutes(t *testing.T) {
	h, _ := testHelper(t)
	mux := h.Routes()
	if mux == nil {
		t.Fatal("Routes() should return a non-nil handler")
	}
}

func TestFormatFlag(t *testing.T) {
	f := &model.Flag{
		Handle:  "flag_abc12",
		Name:    "test",
		Type:    model.FlagTypeBoolean,
		Enabled: true,
		Tags:    []string{"prod", "beta"},
	}
	out := formatFlag(f)
	if !strings.Contains(out, "handle=flag_abc12") {
		t.Error("should contain handle")
	}
	if !strings.Contains(out, "tags=prod,beta") {
		t.Error("should contain tags")
	}
}

func TestFormatEvaluateResult(t *testing.T) {
	r := model.EvaluateResult{
		Handle:  "flag_abc12",
		Enabled: true,
		Variant: "red",
		Reason:  "variant selected: red",
	}
	out := formatEvaluateResult(r)
	if !strings.Contains(out, "variant=red") {
		t.Error("should contain variant")
	}
}

func TestWantsJSON(t *testing.T) {
	req := httptest.NewRequest("GET", "/flags?format=json", nil)
	if !wantsJSON(req) {
		t.Error("format=json should return true")
	}

	req2 := httptest.NewRequest("GET", "/flags", nil)
	req2.Header.Set("Accept", "application/json")
	if !wantsJSON(req2) {
		t.Error("Accept: application/json should return true")
	}

	req3 := httptest.NewRequest("GET", "/flags", nil)
	if wantsJSON(req3) {
		t.Error("no JSON indicators should return false")
	}
}

func TestExtractToken(t *testing.T) {
	req := httptest.NewRequest("GET", "/flags", nil)
	req.Header.Set("Authorization", "Bearer fk_test123")
	token := extractToken(req)
	if token != "fk_test123" {
		t.Errorf("expected fk_test123, got %s", token)
	}

	req2 := httptest.NewRequest("GET", "/flags", nil)
	if extractToken(req2) != "" {
		t.Error("missing header should return empty string")
	}

	req3 := httptest.NewRequest("GET", "/flags", nil)
	req3.Header.Set("Authorization", "Basic abc")
	if extractToken(req3) != "" {
		t.Error("non-bearer auth should return empty string")
	}
}

func TestFullFlowWithServer(t *testing.T) {
	h, token := testHelper(t)
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	// Create flag
	body := bytes.NewReader([]byte("name=integration_test&type=boolean&enabled=true"))
	req, _ := http.NewRequest("POST", srv.URL+"/flags", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List flags
	req2, _ := http.NewRequest("GET", srv.URL+"/flags", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()
}
