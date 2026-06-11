package daemonapp

import (
	"encoding/json"
	"net/http"
)

const (
	errorBadRequest       = "bad_request"
	errorAgentNotFound    = "agent_not_found"
	errorSessionNotFound  = "session_not_found"
	errorSessionBusy      = "session_busy"
	errorBackendFailed    = "backend_failed"
	errorInternal         = "internal_error"
	errorAgentMismatch    = "agent_session_mismatch"
	defaultAgentID        = "codex"
	serviceVersion        = "0.1.0"
	defaultSessionTitle   = "新对话"
	maxSessionTitleRunes  = 80
	autoSessionTitleRunes = 30
)

// apiError 是 MeshWiki service 统一 JSON 错误格式。
type apiError struct {
	Error apiErrorBody `json:"error"`
}

// apiErrorBody 描述错误码、展示信息和可选上下文。
type apiErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func writeAPIError(w http.ResponseWriter, status int, code string, message string, details map[string]any) {
	writeJSON(w, status, apiError{Error: apiErrorBody{Code: code, Message: message, Details: details}})
}

func writeBadRequest(w http.ResponseWriter, message string) {
	writeAPIError(w, http.StatusBadRequest, errorBadRequest, message, nil)
}

func writeInternalError(w http.ResponseWriter, err error) {
	writeAPIError(w, http.StatusInternalServerError, errorInternal, err.Error(), nil)
}

func jsonString(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}
