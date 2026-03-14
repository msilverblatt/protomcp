package playground

import (
	"encoding/json"
	"net/http"
)

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	setCORS(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	tools, err := s.engine.ListTools(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tools)
}

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	resources, err := s.engine.ListResources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resources)
}

func (s *Server) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	prompts, err := s.engine.ListPrompts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prompts)
}

func (s *Server) handleCallTool(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.engine.CallTool(r.Context(), req.Name, req.Args)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"result":         result.Result,
		"duration_ms":    result.Duration.Milliseconds(),
		"tools_enabled":  result.ToolsEnabled,
		"tools_disabled": result.ToolsDisabled,
	})
}

func (s *Server) handleReadResource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URI string `json:"uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.engine.ReadResource(r.Context(), req.URI)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetPrompt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := s.engine.GetPrompt(r.Context(), req.Name, req.Arguments)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if err := s.engine.Reload(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Broadcast reload event
	s.hub.Broadcast(Event{
		Type: "reload",
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	entries := s.engine.Trace().Entries()
	writeJSON(w, http.StatusOK, entries)
}
