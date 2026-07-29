package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// MCPRequest represents a Model Context Protocol request.
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse represents a Model Context Protocol response.
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPCallErr `json:"error,omitempty"`
}

type MCPCallErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handleMCP implements a minimal MCP endpoint for chat client integrations.
func (h *Handler) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, r, http.StatusMethodNotAllowed, "method not allowed", "use POST for MCP requests")
		return
	}

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, MCPResponse{
			JSONRPC: "2.0",
			Error:   &MCPCallErr{Code: -32700, Message: "parse error"},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "initialize":
		writeJSON(w, http.StatusOK, MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "flagkit",
					"version": "0.1.0",
				},
			},
		})

	case "tools/list":
		writeJSON(w, http.StatusOK, MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": []map[string]interface{}{
					{
						"name":        "list_flags",
						"description": "List all feature flags in the workspace",
						"inputSchema": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{},
						},
					},
					{
						"name":        "evaluate_flag",
						"description": "Evaluate a feature flag for a given context",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"handle": map[string]interface{}{
									"type":        "string",
									"description": "The flag handle (e.g. flag_k7m2q)",
								},
								"context": map[string]interface{}{
									"type":        "string",
									"description": "Context key for evaluation (e.g. user ID)",
								},
							},
							"required": []string{"handle"},
						},
					},
				},
			},
		})

	case "tools/call":
		params, _ := json.Marshal(req.Params)
		var p struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		json.Unmarshal(params, &p)

		switch p.Name {
		case "list_flags":
			// Requires auth — return help text
			writeJSON(w, http.StatusOK, MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": "Use GET /flags with Authorization: Bearer <token> to list flags. See /help for auth instructions.",
						},
					},
				},
			})

		case "evaluate_flag":
			handle := p.Arguments["handle"]
			ctx := p.Arguments["context"]
			writeJSON(w, http.StatusOK, MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": "Use POST /flags/" + handle + "/evaluate with body context=" + ctx + " and Authorization: Bearer <token>. See /help for auth instructions.",
						},
					},
				},
			})

		default:
			writeJSON(w, http.StatusOK, MCPResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &MCPCallErr{Code: -32601, Message: "unknown tool: " + p.Name},
			})
		}

	default:
		if strings.HasPrefix(req.Method, "notifications/") {
			// Notifications don't need a response
			return
		}
		writeJSON(w, http.StatusOK, MCPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &MCPCallErr{Code: -32601, Message: "method not found: " + req.Method},
		})
	}
}
