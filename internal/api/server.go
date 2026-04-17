package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-hermes-agent/internal/app"
	"go-hermes-agent/internal/gateway"
	"go-hermes-agent/internal/multiagent"
	"go-hermes-agent/internal/store"
)

// Server exposes the authenticated HTTP API and gateway webhook routes.
type Server struct {
	app *app.App
}

// New creates an HTTP API server bound to the supplied application container.
func New(app *app.App) *Server {
	return &Server{app: app}
}

// Handler returns the top-level HTTP handler with all API and gateway routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/auth/login", s.handleLogin)
	mux.Handle("/v1/chat", s.authMiddleware(http.HandlerFunc(s.handleChat)))
	mux.Handle("/v1/context", s.authMiddleware(http.HandlerFunc(s.handleContext)))
	mux.Handle("/v1/multiagent/plan", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentPlan)))
	mux.Handle("/v1/multiagent/run", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentRun)))
	mux.Handle("/v1/multiagent/traces", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentTraces)))
	mux.Handle("/v1/multiagent/traces/summary", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentTraceSummary)))
	mux.Handle("/v1/multiagent/traces/verifiers", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentVerifierSummary)))
	mux.Handle("/v1/multiagent/traces/failures", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentTraceFailures)))
	mux.Handle("/v1/multiagent/traces/hotspots", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentTraceHotspots)))
	mux.Handle("/v1/multiagent/replay", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentReplay)))
	mux.Handle("/v1/multiagent/resume", s.authMiddleware(http.HandlerFunc(s.handleMultiAgentResume)))
	mux.Handle("/v1/memory", s.authMiddleware(http.HandlerFunc(s.handleMemory)))
	mux.Handle("/v1/models", s.authMiddleware(http.HandlerFunc(s.handleModels)))
	mux.Handle("/v1/models/discover", s.authMiddleware(http.HandlerFunc(s.handleDiscoverModels)))
	mux.Handle("/v1/models/switch", s.authMiddleware(http.HandlerFunc(s.handleModelSwitch)))
	mux.Handle("/v1/sessions", s.authMiddleware(http.HandlerFunc(s.handleSessions)))
	mux.Handle("/v1/history", s.authMiddleware(http.HandlerFunc(s.handleHistory)))
	mux.Handle("/v1/search", s.authMiddleware(http.HandlerFunc(s.handleSearch)))
	mux.Handle("/v1/audit", s.authMiddleware(http.HandlerFunc(s.handleAudit)))
	mux.Handle("/v1/audit/execution", s.authMiddleware(http.HandlerFunc(s.handleExecutionAudit)))
	mux.Handle("/v1/audit/execution/profiles", s.authMiddleware(http.HandlerFunc(s.handleExecutionProfileAudit)))
	mux.Handle("/v1/extensions", s.authMiddleware(http.HandlerFunc(s.handleExtensions)))
	mux.Handle("/v1/extensions/hooks", s.authMiddleware(http.HandlerFunc(s.handleExtensionHooks)))
	mux.Handle("/v1/extensions/refresh", s.authMiddleware(http.HandlerFunc(s.handleRefreshExtensions)))
	mux.Handle("/v1/extensions/state", s.authMiddleware(http.HandlerFunc(s.handleExtensionState)))
	mux.Handle("/v1/extensions/validate", s.authMiddleware(http.HandlerFunc(s.handleExtensionValidate)))
	mux.Handle("/v1/tools", s.authMiddleware(http.HandlerFunc(s.handleTools)))
	mux.Handle("/v1/tools/execute", s.authMiddleware(http.HandlerFunc(s.handleExecuteTool)))
	gateway.RegisterPlatformRoutes(mux, gateway.BuiltInAdapters(s.app)...)
	return mux
}

// ListenAndServe starts the HTTP server and stops it when the context is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	server := &http.Server{
		Addr:         s.app.Config.ListenAddr,
		Handler:      s.Handler(),
		ReadTimeout:  time.Duration(s.app.Config.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(s.app.Config.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(s.app.Config.Server.IdleTimeoutSeconds) * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("hermes-go listening on %s", s.app.Config.ListenAddr)
	return server.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": s.app.Config.AppName})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	token, err := s.app.Auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	claims := usernameFromContext(r.Context())
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}
	budget, err := s.app.EstimateContextBudget(r.Context(), claims, req.Prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	response, err := s.app.Chat(r.Context(), claims, req.Prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"response": response, "context": budget})
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	username := usernameFromContext(r.Context())
	prompt := strings.TrimSpace(r.URL.Query().Get("prompt"))
	if prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}
	budget, err := s.app.EstimateContextBudget(r.Context(), username, prompt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, budget)
}

func (s *Server) handleMultiAgentPlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := usernameFromContext(r.Context())
	var req struct {
		Objective string            `json:"objective"`
		Tasks     []multiagent.Task `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	plan, err := s.app.BuildMultiAgentPlan(r.Context(), username, req.Objective, req.Tasks)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleMultiAgentRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := usernameFromContext(r.Context())
	var plan multiagent.Plan
	if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	results, aggregate, err := s.app.RunMultiAgentPlan(r.Context(), username, plan)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plan":      plan,
		"results":   results,
		"aggregate": aggregate,
	})
}

func (s *Server) handleMultiAgentTraces(w http.ResponseWriter, r *http.Request) {
	filters, err := multiAgentTraceFiltersFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters.Username = usernameFromContext(r.Context())
	records, err := s.app.Store.ListMultiAgentTraces(r.Context(), filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleMultiAgentTraceSummary(w http.ResponseWriter, r *http.Request) {
	filters, err := multiAgentTraceFiltersFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters.Username = usernameFromContext(r.Context())
	records, err := s.app.Store.SummarizeMultiAgentTraces(r.Context(), filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleMultiAgentTraceFailures(w http.ResponseWriter, r *http.Request) {
	filters, err := multiAgentTraceFiltersFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters.Username = usernameFromContext(r.Context())
	records, err := s.app.Store.ListMultiAgentTraceFailures(r.Context(), filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleMultiAgentVerifierSummary(w http.ResponseWriter, r *http.Request) {
	filters, err := multiAgentTraceFiltersFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters.Username = usernameFromContext(r.Context())
	records, err := s.app.Store.SummarizeMultiAgentVerifierResults(r.Context(), filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleMultiAgentTraceHotspots(w http.ResponseWriter, r *http.Request) {
	filters, err := multiAgentTraceFiltersFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	filters.Username = usernameFromContext(r.Context())
	records, err := s.app.Store.ListMultiAgentTraceHotspots(r.Context(), filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleMultiAgentReplay(w http.ResponseWriter, r *http.Request) {
	username := usernameFromContext(r.Context())
	raw := strings.TrimSpace(r.URL.Query().Get("child_session_id"))
	if raw == "" {
		http.Error(w, "child_session_id is required", http.StatusBadRequest)
		return
	}
	childSessionID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || childSessionID <= 0 {
		http.Error(w, "invalid child_session_id", http.StatusBadRequest)
		return
	}
	payload, err := s.app.ReplayMultiAgentChild(r.Context(), username, childSessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleMultiAgentResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := usernameFromContext(r.Context())
	var req struct {
		ChildSessionID int64    `json:"child_session_id"`
		AllowedTools   []string `json:"allowed_tools"`
		HistoryWindow  int      `json:"history_window"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	result, err := s.app.ResumeMultiAgentChild(r.Context(), username, req.ChildSessionID, req.AllowedTools, req.HistoryWindow)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func multiAgentTraceFiltersFromRequest(r *http.Request) (store.MultiAgentTraceFilters, error) {
	filters := store.MultiAgentTraceFilters{Limit: 50}
	if raw := strings.TrimSpace(r.URL.Query().Get("parent_session_id")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return store.MultiAgentTraceFilters{}, fmt.Errorf("invalid parent_session_id")
		}
		filters.ParentSessionID = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("child_session_id")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return store.MultiAgentTraceFilters{}, fmt.Errorf("invalid child_session_id")
		}
		filters.ChildSessionID = value
	}
	filters.TaskID = strings.TrimSpace(r.URL.Query().Get("task_id"))
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return store.MultiAgentTraceFilters{}, fmt.Errorf("invalid limit")
		}
		filters.Limit = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return store.MultiAgentTraceFilters{}, fmt.Errorf("invalid offset")
		}
		filters.Offset = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return store.MultiAgentTraceFilters{}, fmt.Errorf("invalid from")
		}
		filters.FromTime = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return store.MultiAgentTraceFilters{}, fmt.Errorf("invalid to")
		}
		filters.ToTime = value
	}
	return filters, nil
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	username := usernameFromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		snapshot, err := s.app.Memory.Read(r.Context(), username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	case http.MethodPost:
		var req struct {
			Target  string `json:"target"`
			Action  string `json:"action"`
			Content string `json:"content"`
			Match   string `json:"match"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		snapshot, err := s.app.Memory.Write(r.Context(), username, req.Target, req.Action, req.Content, req.Match)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = s.app.Store.WriteAudit(r.Context(), username, "memory_write", "api")
		writeJSON(w, http.StatusOK, snapshot)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	current := s.app.CurrentLLM()
	writeJSON(w, http.StatusOK, map[string]any{
		"current_profile": s.app.CurrentModelProfile(),
		"current":         current,
		"profiles":        s.app.ListModelProfiles(),
	})
}

func (s *Server) handleDiscoverModels(w http.ResponseWriter, r *http.Request) {
	discovered, err := s.app.DiscoverLocalModels(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": discovered})
}

func (s *Server) handleModelSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := usernameFromContext(r.Context())
	var req struct {
		Profile     string `json:"profile"`
		Model       string `json:"model"`
		BaseURL     string `json:"base_url"`
		Provider    string `json:"provider"`
		APIKey   string `json:"api_key"`
		DisplayName string `json:"display_name"`
		Local       bool   `json:"local"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Model) != "" {
		profileName := strings.TrimSpace(req.Profile)
		if profileName == "" {
			profileName = "custom-" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(req.Model)), ":", "-")
		}
		llmCfg := s.app.CurrentLLM()
		llmCfg.Model = strings.TrimSpace(req.Model)
		if strings.TrimSpace(req.BaseURL) != "" {
			llmCfg.BaseURL = strings.TrimSpace(req.BaseURL)
		}
		if strings.TrimSpace(req.Provider) != "" {
			llmCfg.Provider = strings.TrimSpace(req.Provider)
		}
		llmCfg.APIKey = strings.TrimSpace(req.APIKey)
		if strings.TrimSpace(req.DisplayName) != "" {
			llmCfg.DisplayName = strings.TrimSpace(req.DisplayName)
		}
		llmCfg.Local = req.Local
		if err := s.app.SwitchModelConfig(r.Context(), username, profileName, llmCfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":              true,
			"current_profile": s.app.CurrentModelProfile(),
			"current":         s.app.CurrentLLM(),
		})
		return
	}
	profileName := strings.TrimSpace(req.Profile)
	if profileName == "" {
		http.Error(w, "profile or model is required", http.StatusBadRequest)
		return
	}
	if resolved, ok := s.app.ResolveModelProfile(profileName); ok {
		profileName = resolved
	}
	if err := s.app.SwitchModelProfile(r.Context(), username, profileName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"current_profile": s.app.CurrentModelProfile(),
		"current":         s.app.CurrentLLM(),
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	claims := usernameFromContext(r.Context())
	sessions, err := s.app.Store.ListSessions(r.Context(), claims, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	claims := usernameFromContext(r.Context())
	sessionLimit := 20
	sessionOffset := 0
	messageLimit := 20
	messageOffset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		sessionLimit = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
		sessionOffset = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("messages_limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			http.Error(w, "invalid messages_limit", http.StatusBadRequest)
			return
		}
		messageLimit = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("messages_offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			http.Error(w, "invalid messages_offset", http.StatusBadRequest)
			return
		}
		messageOffset = value
	}
	sessions, err := s.app.Store.ListSessionsPage(r.Context(), claims, sessionLimit, sessionOffset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type sessionHistory struct {
		Session  any `json:"session"`
		Messages any `json:"messages"`
	}
	history := make([]sessionHistory, 0, len(sessions))
	for _, session := range sessions {
		messages, err := s.app.Store.GetMessagesPage(r.Context(), session.ID, messageLimit, messageOffset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		history = append(history, sessionHistory{
			Session:  session,
			Messages: messages,
		})
	}
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	claims := usernameFromContext(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}
	filters := store.SearchFilters{
		Username: claims,
		Query:    query,
		Role:     strings.TrimSpace(r.URL.Query().Get("role")),
		Limit:    20,
	}
	if sessionIDRaw := strings.TrimSpace(r.URL.Query().Get("session_id")); sessionIDRaw != "" {
		sessionID, err := strconv.ParseInt(sessionIDRaw, 10, 64)
		if err != nil {
			http.Error(w, "invalid session_id", http.StatusBadRequest)
			return
		}
		filters.SessionID = sessionID
	}
	if fromRaw := strings.TrimSpace(r.URL.Query().Get("from")); fromRaw != "" {
		fromTime, err := time.Parse(time.RFC3339, fromRaw)
		if err != nil {
			http.Error(w, "invalid from time", http.StatusBadRequest)
			return
		}
		filters.FromTime = fromTime
	}
	if toRaw := strings.TrimSpace(r.URL.Query().Get("to")); toRaw != "" {
		toTime, err := time.Parse(time.RFC3339, toRaw)
		if err != nil {
			http.Error(w, "invalid to time", http.StatusBadRequest)
			return
		}
		filters.ToTime = toTime
	}
	results, err := s.app.Store.SearchMessages(r.Context(), filters)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	username := usernameFromContext(r.Context())
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	limit := 50
	offset := 0
	var fromTime time.Time
	var toTime time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
		offset = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid from time", http.StatusBadRequest)
			return
		}
		fromTime = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid to time", http.StatusBadRequest)
			return
		}
		toTime = parsed
	}
	records, err := s.app.Store.ListAuditFiltered(r.Context(), store.AuditFilters{
		Username: username,
		Action:   action,
		FromTime: fromTime,
		ToTime:   toTime,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleExecutionAudit(w http.ResponseWriter, r *http.Request) {
	username := usernameFromContext(r.Context())
	limit := 50
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
		offset = value
	}
	records, err := s.app.Store.ListAuditFiltered(r.Context(), store.AuditFilters{
		Username: username,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := make([]store.AuditRecord, 0, len(records))
	for _, record := range records {
		if strings.HasPrefix(record.Action, "system_exec_") {
			filtered = append(filtered, record)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (s *Server) handleExecutionProfileAudit(w http.ResponseWriter, r *http.Request) {
	username := usernameFromContext(r.Context())
	var fromTime time.Time
	var toTime time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("from")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid from time", http.StatusBadRequest)
			return
		}
		fromTime = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("to")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid to time", http.StatusBadRequest)
			return
		}
		toTime = parsed
	}
	records, err := s.app.Store.ListAuditFiltered(r.Context(), store.AuditFilters{
		Username: username,
		Action:   "system_exec_profile_%",
		FromTime: fromTime,
		ToTime:   toTime,
		Limit:    200,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	summary, err := s.app.Store.SummarizeAuditActions(r.Context(), store.AuditFilters{
		Username: username,
		Action:   "system_exec_profile_%",
		FromTime: fromTime,
		ToTime:   toTime,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type executionProfileSummary struct {
		Profile string `json:"profile"`
		Total   int    `json:"total"`
		Success int    `json:"success"`
		Denied  int    `json:"denied"`
	}
	profileTotals := map[string]*executionProfileSummary{}
	for _, record := range records {
		profile := extractAuditDetailValue(record.Detail, "profile")
		if profile == "" {
			profile = "unknown"
		}
		entry, ok := profileTotals[profile]
		if !ok {
			entry = &executionProfileSummary{Profile: profile}
			profileTotals[profile] = entry
		}
		entry.Total++
		switch {
		case strings.HasSuffix(record.Action, "_success"):
			entry.Success++
		case strings.HasSuffix(record.Action, "_denied"):
			entry.Denied++
		}
	}
	profileSummary := make([]executionProfileSummary, 0, len(profileTotals))
	for _, entry := range profileTotals {
		profileSummary = append(profileSummary, *entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"records":         records,
		"action_summary":  summary,
		"profile_summary": profileSummary,
		"category":        "system_exec_profile",
	})
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.app.Tools.List())
}

func (s *Server) handleExtensions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.app.Extensions.Summary())
}

func (s *Server) handleExtensionHooks(w http.ResponseWriter, r *http.Request) {
	username := usernameFromContext(r.Context())
	limit, offset := parseLimitOffset(r, 50)
	records, err := s.app.Store.ListExtensionHookRuns(r.Context(), store.ExtensionHookFilters{
		Username: username,
		Kind:     strings.TrimSpace(r.URL.Query().Get("kind")),
		Name:     strings.TrimSpace(r.URL.Query().Get("name")),
		Phase:    strings.TrimSpace(r.URL.Query().Get("phase")),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleRefreshExtensions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.app.Extensions.Discover(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := s.app.Extensions.Register(s.app.Tools); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Extensions.Summary())
}

func (s *Server) handleExtensionState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := usernameFromContext(r.Context())
	var req struct {
		Kind    string `json:"kind"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if err := s.app.Extensions.SetEnabled(r.Context(), username, req.Kind, req.Name, req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.app.Extensions.Register(s.app.Tools); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, s.app.Extensions.Summary())
}

func (s *Server) handleExtensionValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := usernameFromContext(r.Context())
	var req struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	result, err := s.app.Extensions.Validate(r.Context(), username, req.Kind, req.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleExecuteTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := usernameFromContext(r.Context())
	var req struct {
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Input == nil {
		req.Input = make(map[string]any)
	}
	req.Input["username"] = username
	result, err := s.app.Tools.Execute(r.Context(), req.Name, req.Input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type ctxKey string

const usernameKey ctxKey = "username"

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		claims, err := s.app.Auth.ParseToken(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), usernameKey, claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func usernameFromContext(ctx context.Context) string {
	username, _ := ctx.Value(usernameKey).(string)
	return username
}

func parseLimitOffset(r *http.Request, defaultLimit int) (int, int) {
	limit := defaultLimit
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			limit = value
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			offset = value
		}
	}
	return limit, offset
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func extractAuditDetailValue(detail, key string) string {
	prefix := key + "="
	for _, part := range strings.Fields(strings.TrimSpace(detail)) {
		if strings.HasPrefix(part, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(part, prefix))
		}
	}
	return ""
}
