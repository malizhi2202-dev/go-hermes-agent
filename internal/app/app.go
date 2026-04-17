package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"go-hermes-agent/internal/auth"
	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/contextengine"
	"go-hermes-agent/internal/execution"
	"go-hermes-agent/internal/extensions"
	"go-hermes-agent/internal/llm"
	"go-hermes-agent/internal/memory"
	"go-hermes-agent/internal/models"
	"go-hermes-agent/internal/multiagent"
	"go-hermes-agent/internal/prompting"
	"go-hermes-agent/internal/store"
	"go-hermes-agent/internal/tools"
)

type App struct {
	mu         sync.RWMutex
	Config     config.Config
	Store      *store.Store
	Auth       *auth.Service
	LLM        *llm.Client
	Tools      *tools.Registry
	Exec       execution.Policy
	Runner     *execution.Executor
	Extensions *extensions.Manager
	Memory     *memory.Manager
	Compressor *contextengine.Compressor
	Prompts    *prompting.Builder
	Auxiliary  *llm.AuxiliaryRouter
	MultiAgent *multiagent.Orchestrator
}

// ContextBudget describes how much recent history and summary state were used
// to build the next prompt.
type ContextBudget struct {
	Model                   string `json:"model"`
	HistoryWindowMessages   int    `json:"history_window_messages"`
	HistoryMessagesUsed     int    `json:"history_messages_used"`
	CompressionEnabled      bool   `json:"compression_enabled"`
	SummaryStrategy         string `json:"summary_strategy"`
	Compressed              bool   `json:"compressed"`
	CompressedMessages      int    `json:"compressed_messages"`
	CompressionSummaryChars int    `json:"compression_summary_chars"`
	PersistedSummaryChars   int    `json:"persisted_summary_chars"`
	TailMessagesUsed        int    `json:"tail_messages_used"`
	SystemBlocksUsed        int    `json:"system_blocks_used"`
	PromptChars             int    `json:"prompt_chars"`
	MaxPromptChars          int    `json:"max_prompt_chars"`
}

// ChatResult is the structured result of one chat turn.
type ChatResult struct {
	SessionID int64                 `json:"session_id"`
	Model     string                `json:"model"`
	Prompt    string                `json:"prompt"`
	Response  string                `json:"response"`
	Plan      prompting.BuildResult `json:"plan"`
}

// ResumeBasis describes the exact trace step used to rebuild a resumed child task.
type ResumeToolState struct {
	Iteration int    `json:"iteration"`
	Tool      string `json:"tool"`
	Output    string `json:"output,omitempty"`
}

// ResumeBasis describes the exact trace step used to rebuild a resumed child task.
type ResumeBasis struct {
	LastIteration             int               `json:"last_iteration"`
	LastSuccessfulTool        string            `json:"last_successful_tool,omitempty"`
	LastSuccessfulOut         string            `json:"last_successful_output,omitempty"`
	LastFailedTool            string            `json:"last_failed_tool,omitempty"`
	LastFailedError           string            `json:"last_failed_error,omitempty"`
	LastFailedInput           map[string]any    `json:"last_failed_input,omitempty"`
	RecoveredHistoryMessage   int               `json:"recovered_history_messages,omitempty"`
	RecoveredToolStates       []ResumeToolState `json:"recovered_tool_states,omitempty"`
	LastSnapshot              map[string]any    `json:"last_snapshot,omitempty"`
	LastSnapshotPrompt        string            `json:"last_snapshot_prompt,omitempty"`
	LastSnapshotHistoryLen    int               `json:"last_snapshot_history_len,omitempty"`
	LastSnapshotHistory       []llm.Message     `json:"last_snapshot_history,omitempty"`
	LastSnapshotNextIteration int               `json:"last_snapshot_next_iteration,omitempty"`
	LastSnapshotRuntime       string            `json:"last_snapshot_runtime,omitempty"`
	LastSnapshotToolRisks     []string          `json:"last_snapshot_tool_risks,omitempty"`
}

type childLoopSeed struct {
	CurrentPrompt string
	NextIteration int
	Runtime       string
	ToolRisks     []string
}

// New wires together the application container and its core dependencies.
func New(cfg config.Config) (*App, error) {
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	authService, err := auth.NewService(cfg, st)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	registry := tools.New()
	var promptCache *prompting.Cache
	if cfg.Prompting.CacheEnabled {
		promptCache = prompting.NewCache(time.Duration(cfg.Prompting.CacheTTLMinutes) * time.Minute)
	}
	application := &App{
		Config:     cfg,
		Store:      st,
		Auth:       authService,
		LLM:        llm.New(cfg),
		Tools:      registry,
		Exec:       execution.DefaultPolicy(),
		Runner:     execution.NewExecutor(cfg.Execution),
		Memory:     memory.NewManager(memory.NewFileProvider(cfg), cfg.Memory.Enabled),
		Compressor: contextengine.New(cfg.Context),
		MultiAgent: multiagent.NewOrchestrator(multiagent.DefaultPolicy()),
		Extensions: extensions.NewManager(
			cfg,
			st.WriteAudit,
			func(ctx context.Context) ([]extensions.ExtensionStateRecord, error) {
				states, err := st.ListExtensionStates(ctx)
				if err != nil {
					return nil, err
				}
				result := make([]extensions.ExtensionStateRecord, 0, len(states))
				for _, state := range states {
					result = append(result, extensions.ExtensionStateRecord{
						Kind:    state.Kind,
						Name:    state.Name,
						Enabled: state.Enabled,
						Hash:    state.Hash,
					})
				}
				return result, nil
			},
			st.UpsertExtensionState,
			func(ctx context.Context, record extensions.ExtensionHookRecord) error {
				return st.InsertExtensionHookRun(ctx, store.ExtensionHookRecord{
					Username: record.Username,
					Kind:     record.Kind,
					Name:     record.Name,
					Phase:    record.Phase,
					Hook:     record.Hook,
					Status:   record.Status,
					Output:   record.Output,
					Error:    record.Error,
				})
			},
		),
	}
	application.Auxiliary = llm.NewAuxiliaryRouter(func() config.Config {
		application.mu.RLock()
		defer application.mu.RUnlock()
		return application.Config
	})
	application.Prompts = prompting.NewBuilder(prompting.Dependencies{
		PrefetchMemory: func(ctx context.Context, username, prompt string) (string, error) {
			return application.Memory.Prefetch(ctx, username, prompt)
		},
		GetSummary: func(ctx context.Context, username string) (store.ContextSummary, error) {
			return application.Store.GetContextSummary(ctx, username)
		},
		PersistSummary: func(ctx context.Context, username, summary, strategy string) error {
			return application.Store.UpsertContextSummary(ctx, username, summary, strategy)
		},
		ListRecent: func(ctx context.Context, username string, limit int) ([]store.Message, error) {
			return application.Store.ListRecentMessagesByUsername(ctx, username, limit)
		},
		Compress: func(ctx context.Context, existingSummary string, history []llm.Message) contextengine.Result {
			return application.Compressor.Compress(ctx, existingSummary, history)
		},
	}, promptCache)
	application.Compressor.WithSummarizer(func(ctx context.Context, existingSummary string, history []llm.Message, maxChars int) (string, error) {
		if application.Config.Context.SummaryStrategy != "llm" {
			return "", nil
		}
		lines := make([]string, 0, len(history))
		for _, msg := range history {
			if msg.Content == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
		}
		prompt := "Summarize the older conversation context into a compact handoff. Keep facts, decisions, and unresolved items only."
		if existingSummary != "" {
			prompt += "\nPrevious summary:\n" + existingSummary
		}
		if len(lines) > 0 {
			prompt += "\nNew conversation chunk:\n" + strings.Join(lines, "\n")
		}
		summary, _, err := application.Auxiliary.Chat(ctx, "compression", []string{
			"You summarize prior conversation context for handoff.",
			fmt.Sprintf("Return plain text only. Keep under %d characters.", maxChars),
		}, prompt)
		if err != nil {
			return "", err
		}
		return summary, nil
	})
	if err := tools.RegisterBuiltins(registry, tools.BuiltinDeps{
		AppName:       cfg.AppName,
		Model:         cfg.ResolvedLLM().Model,
		ExecEnabled:   cfg.Execution.Enabled,
		MemoryEnabled: cfg.Memory.Enabled,
		ListSessions: func(ctx context.Context, username string, limit int) (any, error) {
			return application.Store.ListSessions(ctx, username, limit)
		},
		Search: func(ctx context.Context, username, query, role string, sessionID int64, fromTime, toTime string, limit int) (any, error) {
			filters := store.SearchFilters{
				Username:  username,
				Query:     query,
				Role:      role,
				SessionID: sessionID,
				Limit:     limit,
			}
			if fromTime != "" {
				if parsed, err := time.Parse(time.RFC3339, fromTime); err == nil {
					filters.FromTime = parsed
				}
			}
			if toTime != "" {
				if parsed, err := time.Parse(time.RFC3339, toTime); err == nil {
					filters.ToTime = parsed
				}
			}
			return application.Store.SearchMessages(ctx, filters)
		},
		ReadMemory: func(ctx context.Context, username string) (any, error) {
			return application.Memory.Read(ctx, username)
		},
		WriteMemory: func(ctx context.Context, username, target, action, content, match string) (any, error) {
			result, err := application.Memory.Write(ctx, username, target, action, content, match)
			if err == nil {
				_ = application.Store.WriteAudit(ctx, username, "memory_write", fmt.Sprintf("target=%s action=%s", target, action))
			}
			return result, err
		},
		ExecuteCommand: func(ctx context.Context, command string, args []string) (string, error) {
			return application.Runner.Execute(ctx, command, args)
		},
		ExecuteProfile: func(ctx context.Context, profile string, vars map[string]string, approved bool, capabilityToken string) (any, error) {
			return application.Runner.ExecuteProfile(ctx, profile, vars, approved, capabilityToken)
		},
		WriteAudit: application.Store.WriteAudit,
	}); err != nil {
		_ = st.Close()
		return nil, err
	}
	if err := application.Extensions.Discover(context.Background()); err != nil {
		_ = st.Close()
		return nil, err
	}
	if err := application.Extensions.Register(registry); err != nil {
		_ = st.Close()
		return nil, err
	}
	return application, nil
}

// Close shuts down the application and closes persistent resources.
func (a *App) Close() error {
	return a.Store.Close()
}

// Chat runs one authenticated chat turn and records the result as a new session.
func (a *App) Chat(ctx context.Context, username, prompt string) (string, error) {
	result, err := a.ChatDetailed(ctx, username, prompt)
	if err != nil {
		return "", err
	}
	return result.Response, nil
}

// ChatDetailed runs one authenticated chat turn and returns the session-linked result.
func (a *App) ChatDetailed(ctx context.Context, username, prompt string) (ChatResult, error) {
	a.mu.RLock()
	client := a.LLM
	model := a.Config.ResolvedLLM().Model
	builder := a.Prompts
	mem := a.Memory
	input := prompting.BuildInput{
		Username:           username,
		Prompt:             prompt,
		Model:              model,
		HistoryWindow:      a.Config.Context.HistoryWindowMessages,
		MaxPromptChars:     a.Config.Context.MaxPromptChars,
		SummaryStrategy:    a.Config.Context.SummaryStrategy,
		CompressionEnabled: a.Config.Context.CompressionEnabled,
	}
	a.mu.RUnlock()
	plan, err := builder.Build(ctx, input)
	if err != nil {
		return ChatResult{}, err
	}
	if plan.Compression.Compressed {
		_ = a.Store.WriteAudit(ctx, username, "context_compress_applied", fmt.Sprintf("compressed_messages=%d", plan.Compression.CompressedMessages))
	}
	response, err := client.ChatWithMessages(ctx, plan.SystemBlocks, plan.History, prompt)
	if err != nil {
		return ChatResult{}, err
	}
	sessionID, err := a.Store.CreateSession(ctx, username, model, prompt, response)
	if err != nil {
		return ChatResult{}, err
	}
	if err := a.Store.AddMessage(ctx, sessionID, "user", prompt); err != nil {
		return ChatResult{}, err
	}
	if err := a.Store.AddMessage(ctx, sessionID, "assistant", response); err != nil {
		return ChatResult{}, err
	}
	if err := mem.SyncTurn(ctx, username, prompt, response); err != nil {
		return ChatResult{}, err
	}
	_ = a.Store.WriteAudit(ctx, username, "chat", fmt.Sprintf("session recorded cache_hit=%t", plan.CacheHit))
	return ChatResult{
		SessionID: sessionID,
		Model:     model,
		Prompt:    prompt,
		Response:  response,
		Plan:      plan,
	}, nil
}

// EstimateContextBudget reports how much context would be injected for a prompt.
func (a *App) EstimateContextBudget(ctx context.Context, username, prompt string) (ContextBudget, error) {
	plan, err := a.InspectPrompt(ctx, username, prompt)
	if err != nil {
		return ContextBudget{}, err
	}
	return ContextBudget{
		Model:                   plan.Model,
		HistoryWindowMessages:   plan.HistoryWindowMessages,
		HistoryMessagesUsed:     plan.HistoryMessagesUsed,
		CompressionEnabled:      a.Config.Context.CompressionEnabled,
		SummaryStrategy:         a.Config.Context.SummaryStrategy,
		Compressed:              plan.Compression.Compressed,
		CompressedMessages:      plan.Compression.CompressedMessages,
		CompressionSummaryChars: plan.Compression.SummaryChars,
		PersistedSummaryChars:   len(plan.PersistedSummary),
		TailMessagesUsed:        plan.Compression.TailMessagesUsed,
		SystemBlocksUsed:        plan.SystemBlocksUsed,
		PromptChars:             plan.PromptChars,
		MaxPromptChars:          plan.MaxPromptChars,
	}, nil
}

// InspectPrompt returns the fully assembled prompt plan used for one chat turn.
func (a *App) InspectPrompt(ctx context.Context, username, prompt string) (prompting.BuildResult, error) {
	a.mu.RLock()
	builder := a.Prompts
	input := prompting.BuildInput{
		Username:           username,
		Prompt:             prompt,
		Model:              a.Config.ResolvedLLM().Model,
		HistoryWindow:      a.Config.Context.HistoryWindowMessages,
		MaxPromptChars:     a.Config.Context.MaxPromptChars,
		SummaryStrategy:    a.Config.Context.SummaryStrategy,
		CompressionEnabled: a.Config.Context.CompressionEnabled,
	}
	a.mu.RUnlock()
	return builder.Build(ctx, input)
}

// PromptCacheStats returns local prompt cache counters.
func (a *App) PromptCacheStats() prompting.CacheStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Prompts.CacheStats()
}

// ClearPromptCache removes all local prompt plan cache entries.
func (a *App) ClearPromptCache() {
	a.mu.RLock()
	defer a.mu.RUnlock()
	a.Prompts.ClearCache()
}

// AuxiliaryInfo returns the resolved auxiliary model for one side-task.
func (a *App) AuxiliaryInfo(task string) (llm.AuxiliaryResolution, error) {
	a.mu.RLock()
	router := a.Auxiliary
	a.mu.RUnlock()
	return router.Resolve(task)
}

// AuxiliaryChat runs one lightweight auxiliary request.
func (a *App) AuxiliaryChat(ctx context.Context, task, prompt string) (string, llm.AuxiliaryResolution, error) {
	a.mu.RLock()
	router := a.Auxiliary
	a.mu.RUnlock()
	return router.Chat(ctx, task, []string{"You are a lightweight auxiliary assistant for Hermes-Go."}, prompt)
}

// CurrentModelMetadata returns lightweight provider-aware metadata for the active model.
func (a *App) CurrentModelMetadata() models.Metadata {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return models.ResolveMetadata(a.Config.ResolvedLLM())
}

// ListModelMetadata returns lightweight metadata for all configured model profiles.
func (a *App) ListModelMetadata() map[string]models.Metadata {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return models.ListProfileMetadata(a.Config.ModelProfiles)
}

// CurrentLLM returns the currently active resolved LLM config.
func (a *App) CurrentLLM() config.LLMConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Config.ResolvedLLM()
}

// CurrentModelProfile returns the active model profile name.
func (a *App) CurrentModelProfile() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Config.CurrentModelProfile
}

// ListModelProfiles returns the configured model profiles.
func (a *App) ListModelProfiles() map[string]config.LLMConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Config.ListModelProfiles()
}

// SwitchModelProfile switches the active model profile by name.
func (a *App) SwitchModelProfile(ctx context.Context, username, profile string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.Config.UseModelProfile(profile); err != nil {
		return err
	}
	a.LLM = llm.New(a.Config)
	_ = a.Store.WriteAudit(ctx, username, "model_switch", fmt.Sprintf("profile=%s model=%s", profile, a.Config.ResolvedLLM().Model))
	return nil
}

// ResolveModelProfile resolves a user-facing alias or profile name.
func (a *App) ResolveModelProfile(name string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return models.DefaultCatalog().ResolveProfile(a.Config.ListModelProfiles(), name)
}

// DiscoverLocalModels discovers local OpenAI-compatible model endpoints.
func (a *App) DiscoverLocalModels(ctx context.Context) ([]models.DiscoveredModel, error) {
	a.mu.RLock()
	profiles := a.Config.ListModelProfiles()
	a.mu.RUnlock()
	return models.DiscoverLocalModels(ctx, profiles)
}

// SwitchModelConfig upserts and activates a model profile config.
func (a *App) SwitchModelConfig(ctx context.Context, username, profileName string, llmCfg config.LLMConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.Config.UpsertModelProfile(profileName, llmCfg); err != nil {
		return err
	}
	a.LLM = llm.New(a.Config)
	_ = a.Store.WriteAudit(ctx, username, "model_switch", fmt.Sprintf("profile=%s model=%s", profileName, a.Config.ResolvedLLM().Model))
	return nil
}

// BuildMultiAgentPlan validates and constructs a multi-agent plan.
func (a *App) BuildMultiAgentPlan(ctx context.Context, username, objective string, tasks []multiagent.Task) (multiagent.Plan, error) {
	plan, err := a.MultiAgent.BuildPlan(objective, tasks)
	if err != nil {
		return multiagent.Plan{}, err
	}
	_ = a.Store.WriteAudit(ctx, username, "multiagent_plan_built", fmt.Sprintf("tasks=%d mode=%s", len(plan.Tasks), plan.Mode))
	return plan, nil
}

// RunMultiAgentPlan executes a validated multi-agent plan and persists parent summaries.
func (a *App) RunMultiAgentPlan(ctx context.Context, username string, plan multiagent.Plan) ([]multiagent.Result, multiagent.Aggregate, error) {
	parentSessionID := plan.ParentSessionID
	if parentSessionID == 0 {
		sessionID, err := a.Store.CreateSessionWithOptions(
			ctx,
			username,
			a.CurrentLLM().Model,
			"[multiagent] "+plan.Objective,
			"multiagent run created",
			store.CreateSessionOptions{Kind: "multiagent_parent"},
		)
		if err != nil {
			return nil, multiagent.Aggregate{}, err
		}
		parentSessionID = sessionID
		if err := retryBusy(func() error {
			return a.Store.AddMessage(ctx, parentSessionID, "user", "[multiagent objective] "+plan.Objective)
		}); err != nil {
			return nil, multiagent.Aggregate{}, err
		}
	}
	results, aggregate, err := a.MultiAgent.Run(ctx, plan, func(runCtx context.Context, task multiagent.Task) multiagent.Result {
		return a.runMultiAgentChildTask(runCtx, username, parentSessionID, task)
	})
	if err != nil {
		return nil, multiagent.Aggregate{}, err
	}
	aggregate.ParentSessionID = parentSessionID
	parentSummary := summarizeMultiAgentAggregate(plan.Objective, aggregate)
	if err := retryBusy(func() error {
		return a.Store.AddMessage(ctx, parentSessionID, "assistant", parentSummary)
	}); err != nil {
		return nil, multiagent.Aggregate{}, err
	}
	_ = a.Store.WriteAudit(ctx, username, "multiagent_run", fmt.Sprintf("tasks=%d completed=%d failed=%d", len(results), aggregate.Completed, aggregate.Failed))
	return results, aggregate, nil
}

// RunGatewayMultiAgent builds and runs a safe default multi-agent plan for a
// gateway-issued objective and returns a compact user-facing summary.
func (a *App) RunGatewayMultiAgent(ctx context.Context, username, objective string) (string, error) {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		return "", fmt.Errorf("objective is required")
	}
	plan, err := a.BuildMultiAgentPlan(ctx, username, objective, []multiagent.Task{
		{
			ID:            "gateway-task",
			Title:         "Gateway delegated objective",
			Goal:          objective,
			HistoryWindow: 6,
			AllowedTools:  []string{"session.history", "session.search", "memory.read"},
		},
	})
	if err != nil {
		return "", err
	}
	results, aggregate, err := a.RunMultiAgentPlan(ctx, username, plan)
	if err != nil {
		return "", err
	}
	lines := []string{summarizeMultiAgentAggregate(objective, aggregate)}
	for _, result := range results {
		lines = append(lines, fmt.Sprintf("[task:%s] %s", result.TaskID, result.Summary))
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) runMultiAgentChildTask(ctx context.Context, username string, parentSessionID int64, task multiagent.Task) multiagent.Result {
	return a.runMultiAgentChildTaskWithSeed(ctx, username, parentSessionID, task, nil)
}

func (a *App) runMultiAgentChildTaskWithSeed(ctx context.Context, username string, parentSessionID int64, task multiagent.Task, seedHistory []llm.Message) multiagent.Result {
	return a.runMultiAgentChildTaskWithSeedAndState(ctx, username, parentSessionID, task, seedHistory, childLoopSeed{})
}

func (a *App) runMultiAgentChildTaskWithSeedAndState(ctx context.Context, username string, parentSessionID int64, task multiagent.Task, seedHistory []llm.Message, seed childLoopSeed) multiagent.Result {
	llmCfg := a.CurrentLLM()
	allowedTools, invalidTools := a.resolveChildAllowedTools(task.AllowedTools)
	prompt, historySnippet := a.buildMultiAgentTaskPrompt(ctx, parentSessionID, task)
	childSessionID, err := a.recordMultiAgentChildSession(ctx, username, parentSessionID, task, prompt, "")
	if err != nil {
		return multiagent.Result{
			TaskID:      task.ID,
			Status:      multiagent.ResultFailed,
			Summary:     err.Error(),
			Risks:       []string{"Failed to create child session."},
			NextActions: []string{"Check SQLite child session creation path."},
		}
	}
	summary, runtime, trace, toolRisks, err := a.tryRunMultiAgentLLMTaskWithState(ctx, username, llmCfg, task, prompt, historySnippet, allowedTools, childSessionID, seedHistory, seed)
	risks := make([]string, 0, 2)
	if len(invalidTools) > 0 {
		risks = append(risks, "Some requested child tools are not available: "+strings.Join(invalidTools, ", "))
	}
	risks = append(risks, toolRisks...)
	if err == nil {
		finalSummary := fmt.Sprintf("[runtime=%s] %s", runtime, summary)
		sessionErr := a.finalizeMultiAgentChildSession(ctx, parentSessionID, childSessionID, task, finalSummary)
		if sessionErr != nil {
			return multiagent.Result{
				TaskID:      task.ID,
				Status:      multiagent.ResultFailed,
				Summary:     sessionErr.Error(),
				Risks:       []string{"Failed to persist child session."},
				NextActions: []string{"Check SQLite session persistence path."},
			}
		}
		if traceErr := a.persistMultiAgentTrace(ctx, username, parentSessionID, childSessionID, task.ID, trace); traceErr != nil {
			risks = append(risks, "Failed to persist child trace: "+traceErr.Error())
		}
		return multiagent.Result{
			TaskID:         task.ID,
			ChildSessionID: childSessionID,
			Status:         multiagent.ResultCompleted,
			Summary:        finalSummary,
			Trace:          trace,
			Risks:          risks,
			NextActions:    []string{"Wire task-specific tool execution when delegated tools are implemented."},
		}
	}

	fallback := fmt.Sprintf("Prepared delegated task %s: %s", task.ID, task.Goal)
	if len(task.AllowedTools) > 0 {
		fallback += fmt.Sprintf(" | allowed_tools=%s", strings.Join(task.AllowedTools, ","))
	}
	if len(task.WriteScopes) > 0 {
		fallback += fmt.Sprintf(" | write_scopes=%s", strings.Join(task.WriteScopes, ","))
	}
	sessionErr := a.finalizeMultiAgentChildSession(ctx, parentSessionID, childSessionID, task, "[runtime=stub] "+fallback)
	if sessionErr != nil {
		return multiagent.Result{
			TaskID:      task.ID,
			Status:      multiagent.ResultFailed,
			Summary:     sessionErr.Error(),
			Risks:       []string{"Failed to persist child session."},
			NextActions: []string{"Check SQLite session persistence path."},
		}
	}
	trace = []multiagent.TraceStep{
		{Iteration: 1, Type: "final", Note: "stub runtime fallback"},
	}
	if traceErr := a.persistMultiAgentTrace(ctx, username, parentSessionID, childSessionID, task.ID, trace); traceErr != nil {
		risks = append(risks, "Failed to persist child trace: "+traceErr.Error())
	}
	return multiagent.Result{
		TaskID:         task.ID,
		ChildSessionID: childSessionID,
		Status:         multiagent.ResultCompleted,
		Summary:        "[runtime=stub] " + fallback,
		Trace:          trace,
		Risks:          append(risks, "Child task runtime fell back to stub execution."),
		NextActions:    []string{"Provide API key or local model endpoint to enable LLM child runtime.", "Attach delegated tool runtime for real task execution."},
	}
}

// ReplayMultiAgentChild reconstructs a child run from the stored session and trace rows.
func (a *App) ReplayMultiAgentChild(ctx context.Context, username string, childSessionID int64) (map[string]any, error) {
	session, err := a.Store.GetSession(ctx, childSessionID)
	if err != nil {
		return nil, err
	}
	if session.Username != username {
		return nil, fmt.Errorf("child session does not belong to user")
	}
	traceRows, err := a.Store.ListMultiAgentTraces(ctx, store.MultiAgentTraceFilters{
		Username:       username,
		ChildSessionID: childSessionID,
		Limit:          200,
	})
	if err != nil {
		return nil, err
	}
	messages, err := a.Store.GetMessagesPage(ctx, childSessionID, 200, 0)
	if err != nil {
		return nil, err
	}
	recoveryHint := "No persisted trace was found. Re-run the child task from the parent plan."
	lastStep := map[string]any{}
	if len(traceRows) > 0 {
		last := traceRows[len(traceRows)-1]
		lastStep = map[string]any{
			"iteration": last.Iteration,
			"type":      last.Type,
			"tool":      last.Tool,
			"error":     last.Error,
			"note":      last.Note,
		}
		switch {
		case strings.TrimSpace(last.Error) != "":
			recoveryHint = fmt.Sprintf("Resume from the last failed tool step for %q and inspect the error before continuing.", last.Tool)
		case last.Type == "final":
			recoveryHint = "This child session already reached a final step. Prefer replay or start a new child task instead of resuming it."
		default:
			recoveryHint = "Resume from the last completed tool step and continue the delegated task with fresh context."
		}
	}
	resumeBasis := deriveResumeBasis(traceRows, messages)
	return map[string]any{
		"session":       session,
		"trace":         traceRows,
		"messages":      messages,
		"last_step":     lastStep,
		"resume_basis":  resumeBasis,
		"recovery_hint": recoveryHint,
	}, nil
}

// ResumeMultiAgentChild creates a follow-up child task from a stored child
// session, re-runs it under the same parent session, and returns the new result.
func (a *App) ResumeMultiAgentChild(ctx context.Context, username string, childSessionID int64, allowedTools []string, historyWindow int) (map[string]any, error) {
	session, err := a.Store.GetSession(ctx, childSessionID)
	if err != nil {
		return nil, err
	}
	if session.Username != username {
		return nil, fmt.Errorf("child session does not belong to user")
	}
	if session.Kind != "multiagent_child" {
		return nil, fmt.Errorf("session %d is not a multiagent child session", childSessionID)
	}
	if !session.ParentSessionID.Valid || session.ParentSessionID.Int64 <= 0 {
		return nil, fmt.Errorf("child session %d has no parent session", childSessionID)
	}
	task, err := rebuildTaskFromChildSession(ctx, a.Store, session, allowedTools, historyWindow)
	if err != nil {
		return nil, err
	}
	traceRows, err := a.Store.ListMultiAgentTraces(ctx, store.MultiAgentTraceFilters{
		Username:       username,
		ChildSessionID: childSessionID,
		Limit:          200,
	})
	if err != nil {
		return nil, err
	}
	messages, err := a.Store.GetMessagesPage(ctx, childSessionID, 200, 0)
	if err != nil {
		return nil, err
	}
	resumeBasis := deriveResumeBasis(traceRows, messages)
	task.ID = task.ID + "-resume"
	task.Context = strings.TrimSpace(task.Context + "\n" + buildResumeContext(childSessionID, resumeBasis))
	if strings.TrimSpace(resumeBasis.LastSnapshotPrompt) != "" {
		task.Goal = resumeBasis.LastSnapshotPrompt
	}
	seedHistory := buildResumeSeedHistory(messages, resumeBasis)
	result := a.runMultiAgentChildTaskWithSeedAndState(ctx, username, session.ParentSessionID.Int64, task, seedHistory, buildResumeLoopSeed(resumeBasis))
	return map[string]any{
		"resumed_from_child_session_id": childSessionID,
		"parent_session_id":             session.ParentSessionID.Int64,
		"resume_basis":                  resumeBasis,
		"result":                        result,
	}, nil
}

func (a *App) tryRunMultiAgentLLMTask(ctx context.Context, username string, llmCfg config.LLMConfig, task multiagent.Task, prompt, historySnippet string, allowedTools []tools.Tool, childSessionID int64, seedHistory []llm.Message) (string, string, []multiagent.TraceStep, []string, error) {
	return a.tryRunMultiAgentLLMTaskWithState(ctx, username, llmCfg, task, prompt, historySnippet, allowedTools, childSessionID, seedHistory, childLoopSeed{})
}

func (a *App) tryRunMultiAgentLLMTaskWithState(ctx context.Context, username string, llmCfg config.LLMConfig, task multiagent.Task, prompt, historySnippet string, allowedTools []tools.Tool, childSessionID int64, seedHistory []llm.Message, seed childLoopSeed) (string, string, []multiagent.TraceStep, []string, error) {
	if strings.TrimSpace(llmCfg.BaseURL) == "" || strings.TrimSpace(llmCfg.Model) == "" {
		return "", "", nil, nil, fmt.Errorf("llm is not configured")
	}
	preferredRuntime := strings.TrimSpace(seed.Runtime)
	if preferredRuntime == "llm" {
		return a.tryRunMultiAgentJSONTask(ctx, username, task, prompt, historySnippet, allowedTools, childSessionID, seedHistory, seed)
	}
	if len(allowedTools) > 0 {
		if summary, runtime, trace, toolRisks, err := a.tryRunMultiAgentToolCallingTask(ctx, username, task, prompt, historySnippet, allowedTools, childSessionID, seedHistory, seed); err == nil {
			return summary, runtime, trace, toolRisks, nil
		}
	}
	return a.tryRunMultiAgentJSONTask(ctx, username, task, prompt, historySnippet, allowedTools, childSessionID, seedHistory, seed)
}

func (a *App) tryRunMultiAgentToolCallingTask(ctx context.Context, username string, task multiagent.Task, prompt, historySnippet string, allowedTools []tools.Tool, childSessionID int64, seedHistory []llm.Message, seed childLoopSeed) (string, string, []multiagent.TraceStep, []string, error) {
	systemBlocks := []string{
		"You are a focused child agent working on one delegated subtask.",
		"Do not delegate further.",
		"When enough information has been gathered, answer with a concise final response.",
	}
	if len(task.WriteScopes) > 0 {
		systemBlocks = append(systemBlocks, "Allowed write scopes: "+strings.Join(task.WriteScopes, ", "))
	}
	if strings.TrimSpace(historySnippet) != "" {
		systemBlocks = append(systemBlocks, "Recent parent session context:\n"+historySnippet)
	}
	history := append([]llm.Message(nil), seedHistory...)
	currentPrompt := prompt
	if strings.TrimSpace(seed.CurrentPrompt) != "" {
		currentPrompt = strings.TrimSpace(seed.CurrentPrompt)
	}
	trace := make([]multiagent.TraceStep, 0, 8)
	toolRisks := append([]string(nil), seed.ToolRisks...)
	toolDefs := buildLLMToolDefinitions(allowedTools)
	startIteration := 1
	if seed.NextIteration > 1 {
		startIteration = seed.NextIteration
	}
	for iteration := startIteration; iteration <= 4; iteration++ {
		trace = append(trace, a.childLoopSnapshot(iteration, task, currentPrompt, history, allowedTools, "llm-toolcalls", toolRisks))
		completion, err := a.LLM.ChatCompletion(ctx, systemBlocks, history, currentPrompt, toolDefs)
		if err != nil {
			return "", "", trace, toolRisks, err
		}
		if err := retryBusy(func() error {
			return a.Store.AddMessage(ctx, childSessionID, "assistant", completion.Message.Content)
		}); err != nil {
			return "", "", trace, toolRisks, err
		}
		if len(completion.Message.ToolCalls) > 0 {
			history = append(history, llm.Message{
				Role:      "assistant",
				Content:   completion.Message.Content,
				ToolCalls: completion.Message.ToolCalls,
			})
			for _, call := range completion.Message.ToolCalls {
				step, risk, toolMsg, execErr := a.executeChildToolCall(ctx, username, iteration, call, allowedTools, childSessionID)
				trace = append(trace, step)
				if risk != "" {
					toolRisks = append(toolRisks, risk)
				}
				if execErr != nil {
					currentPrompt = fmt.Sprintf("Tool %s failed with error: %s. Finish the task with a concise final response.", call.Function.Name, execErr.Error())
					goto nextIteration
				}
				history = append(history, toolMsg)
			}
			currentPrompt = ""
			continue
		}
		if action, parseErr := parseChildAction(completion.Message.Content); parseErr == nil {
			if action.Type == "tool" {
				step, risk, toolMsg, execErr := a.executeChildAction(ctx, username, iteration, action, allowedTools, childSessionID)
				trace = append(trace, step)
				if risk != "" {
					toolRisks = append(toolRisks, risk)
				}
				if execErr != nil {
					currentPrompt = fmt.Sprintf("Tool %s failed with error: %s. Finish the task with a concise final response.", action.Tool, execErr.Error())
				} else {
					history = append(history, llm.NewMessage("assistant", completion.Message.Content), toolMsg)
					currentPrompt = ""
				}
				continue
			}
			if action.Type == "final" {
				trace = append(trace, multiagent.TraceStep{Iteration: iteration, Type: "final", Note: "child loop completed"})
				return strings.TrimSpace(action.Summary), "llm-toolcalls", trace, append(toolRisks, action.Risks...), nil
			}
		}
		trace = append(trace, multiagent.TraceStep{Iteration: iteration, Type: "final", Note: "native tool-calling final response"})
		return strings.TrimSpace(completion.Message.Content), "llm-toolcalls", trace, toolRisks, nil
	nextIteration:
		continue
	}
	return "Child loop reached iteration cap.", "llm-toolcalls", trace, append(toolRisks, "Child reached iteration cap."), nil
}

func (a *App) tryRunMultiAgentJSONTask(ctx context.Context, username string, task multiagent.Task, prompt, historySnippet string, allowedTools []tools.Tool, childSessionID int64, seedHistory []llm.Message, seed childLoopSeed) (string, string, []multiagent.TraceStep, []string, error) {
	systemBlocks := []string{
		"You are a focused child agent working on one delegated subtask.",
		"Do not delegate further. Respond with JSON only.",
		`Use one of these JSON shapes:
{"type":"tool","tool":"session.search","input":{"query":"...","limit":5},"note":"why"}
{"type":"final","summary":"...","risks":["..."],"next_actions":["..."]}`,
	}
	if len(allowedTools) > 0 {
		toolLines := make([]string, 0, len(allowedTools))
		for _, tool := range allowedTools {
			toolLines = append(toolLines, fmt.Sprintf("%s: %s", tool.Name, tool.Description))
		}
		systemBlocks = append(systemBlocks, "Allowed tools for this child:\n"+strings.Join(toolLines, "\n"))
	}
	if len(task.WriteScopes) > 0 {
		systemBlocks = append(systemBlocks, "Allowed write scopes: "+strings.Join(task.WriteScopes, ", "))
	}
	if strings.TrimSpace(historySnippet) != "" {
		systemBlocks = append(systemBlocks, "Recent parent session context:\n"+historySnippet)
	}
	history := append([]llm.Message(nil), seedHistory...)
	currentPrompt := prompt
	if strings.TrimSpace(seed.CurrentPrompt) != "" {
		currentPrompt = strings.TrimSpace(seed.CurrentPrompt)
	}
	trace := make([]multiagent.TraceStep, 0, 8)
	toolRisks := append([]string(nil), seed.ToolRisks...)
	startIteration := 1
	if seed.NextIteration > 1 {
		startIteration = seed.NextIteration
	}
	for iteration := startIteration; iteration <= 4; iteration++ {
		trace = append(trace, a.childLoopSnapshot(iteration, task, currentPrompt, history, allowedTools, "llm", toolRisks))
		response, err := a.LLM.ChatWithMessages(ctx, systemBlocks, history, currentPrompt)
		if err != nil {
			return "", "", trace, toolRisks, err
		}
		if err := retryBusy(func() error {
			return a.Store.AddMessage(ctx, childSessionID, "assistant", response)
		}); err != nil {
			return "", "", trace, toolRisks, err
		}
		history = append(history, llm.NewMessage("user", currentPrompt), llm.NewMessage("assistant", response))
		action, parseErr := parseChildAction(response)
		if parseErr != nil {
			trace = append(trace, multiagent.TraceStep{Iteration: iteration, Type: "final", Note: "model returned non-json final response"})
			return strings.TrimSpace(response), "llm", trace, toolRisks, nil
		}
		if action.Type == "final" {
			trace = append(trace, multiagent.TraceStep{Iteration: iteration, Type: "final", Note: "child loop completed"})
			return strings.TrimSpace(action.Summary), "llm", trace, append(toolRisks, action.Risks...), nil
		}
		if action.Type != "tool" {
			return "", "", trace, toolRisks, fmt.Errorf("unknown child action type %q", action.Type)
		}
		step, risk, toolMsg, execErr := a.executeChildAction(ctx, username, iteration, action, allowedTools, childSessionID)
		trace = append(trace, step)
		if risk != "" {
			toolRisks = append(toolRisks, risk)
		}
		if execErr != nil {
			currentPrompt = fmt.Sprintf("Tool %s failed with error: %s. Return final JSON now.", action.Tool, execErr.Error())
			continue
		}
		history = append(history, toolMsg)
		currentPrompt = fmt.Sprintf("Tool %s returned: %s\nReturn either another tool JSON or a final JSON summary.", action.Tool, toolMsg.Content)
	}
	return "Child loop reached iteration cap.", "llm", trace, append(toolRisks, "Child reached iteration cap."), nil
}

func buildMultiAgentTaskPrompt(task multiagent.Task) string {
	prompt := fmt.Sprintf("Task ID: %s\nTitle: %s\nGoal: %s", task.ID, task.Title, task.Goal)
	if strings.TrimSpace(task.Context) != "" {
		prompt += "\nContext:\n" + strings.TrimSpace(task.Context)
	}
	return prompt
}

func (a *App) buildMultiAgentTaskPrompt(ctx context.Context, parentSessionID int64, task multiagent.Task) (string, string) {
	prompt := buildMultiAgentTaskPrompt(task)
	historyWindow := task.HistoryWindow
	if historyWindow <= 0 {
		historyWindow = 4
	}
	if parentSessionID <= 0 {
		return prompt, ""
	}
	messages, err := a.Store.ListRecentMessagesBySession(ctx, parentSessionID, historyWindow)
	if err != nil || len(messages) == 0 {
		return prompt, ""
	}
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", msg.Role, content))
	}
	return prompt, strings.Join(lines, "\n")
}

func (a *App) resolveChildAllowedTools(requested []string) ([]tools.Tool, []string) {
	if len(requested) == 0 {
		return nil, nil
	}
	resolved := make([]tools.Tool, 0, len(requested))
	invalid := make([]string, 0)
	for _, name := range requested {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tool, ok := a.Tools.Get(name)
		if !ok {
			invalid = append(invalid, name)
			continue
		}
		resolved = append(resolved, tool)
	}
	return resolved, invalid
}

func (a *App) recordMultiAgentChildSession(ctx context.Context, username string, parentSessionID int64, task multiagent.Task, prompt, summary string) (int64, error) {
	var (
		childSessionID int64
		err            error
	)
	err = retryBusy(func() error {
		childSessionID, err = a.Store.CreateSessionWithOptions(
			ctx,
			username,
			a.CurrentLLM().Model,
			prompt,
			summary,
			store.CreateSessionOptions{
				Kind:            "multiagent_child",
				TaskID:          task.ID,
				ParentSessionID: parentSessionID,
			},
		)
		return err
	})
	if err != nil {
		return 0, err
	}
	if err := retryBusy(func() error {
		return a.Store.AddMessage(ctx, childSessionID, "user", prompt)
	}); err != nil {
		return 0, err
	}
	if strings.TrimSpace(summary) != "" {
		if err := retryBusy(func() error {
			return a.Store.AddMessage(ctx, childSessionID, "assistant", summary)
		}); err != nil {
			return 0, err
		}
	}
	if parentSessionID > 0 {
		if err := retryBusy(func() error {
			return a.Store.AddMessage(ctx, parentSessionID, "assistant", fmt.Sprintf("[child:%s session:%d] %s", task.ID, childSessionID, summary))
		}); err != nil {
			return 0, err
		}
	}
	return childSessionID, nil
}

func (a *App) finalizeMultiAgentChildSession(ctx context.Context, parentSessionID, childSessionID int64, task multiagent.Task, summary string) error {
	if err := retryBusy(func() error {
		return a.Store.UpdateSessionResponse(ctx, childSessionID, summary)
	}); err != nil {
		return err
	}
	if err := retryBusy(func() error {
		return a.Store.AddMessage(ctx, childSessionID, "assistant", summary)
	}); err != nil {
		return err
	}
	if parentSessionID > 0 {
		if err := retryBusy(func() error {
			return a.Store.AddMessage(ctx, parentSessionID, "assistant", fmt.Sprintf("[child:%s session:%d] %s", task.ID, childSessionID, summary))
		}); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) persistMultiAgentTrace(ctx context.Context, username string, parentSessionID, childSessionID int64, taskID string, trace []multiagent.TraceStep) error {
	for _, step := range trace {
		var inputJSON string
		var outputJSON string
		var snapshotJSON string
		if step.Input != nil {
			raw, _ := json.Marshal(step.Input)
			inputJSON = string(raw)
		}
		if step.Output != nil {
			raw, _ := json.Marshal(step.Output)
			outputJSON = string(raw)
		}
		if step.Snapshot != nil {
			raw, _ := json.Marshal(step.Snapshot)
			snapshotJSON = string(raw)
		}
		if err := retryBusy(func() error {
			return a.Store.InsertMultiAgentTrace(ctx, store.MultiAgentTraceRecord{
				Username:          username,
				ParentSessionID:   parentSessionID,
				ChildSessionID:    childSessionID,
				TaskID:            taskID,
				Iteration:         step.Iteration,
				Type:              step.Type,
				Tool:              step.Tool,
				InputJSON:         inputJSON,
				OutputJSON:        outputJSON,
				SnapshotJSON:      snapshotJSON,
				Verified:          step.Verified,
				Verifier:          step.Verifier,
				VerificationClass: step.VerificationClass,
				Error:             step.Error,
				Note:              step.Note,
			})
		}); err != nil {
			return err
		}
	}
	return nil
}

func executeToolInput(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return nil, err
	}
	return input, nil
}

func buildLLMToolDefinitions(allowedTools []tools.Tool) []llm.ToolDefinition {
	if len(allowedTools) == 0 {
		return nil
	}
	result := make([]llm.ToolDefinition, 0, len(allowedTools))
	for _, tool := range allowedTools {
		properties := make(map[string]any, len(tool.InputKeys))
		required := make([]string, 0, len(tool.InputKeys))
		for _, key := range tool.InputKeys {
			switch key {
			case "vars":
				properties[key] = map[string]any{
					"type":                 "object",
					"description":          "Key/value variables used to render an execution profile.",
					"additionalProperties": map[string]any{"type": "string"},
				}
			case "approved":
				properties[key] = map[string]any{
					"type":        "boolean",
					"description": "Whether execution was explicitly approved.",
				}
			case "args":
				properties[key] = map[string]any{
					"type":        "array",
					"description": "Command arguments.",
					"items":       map[string]any{"type": "string"},
				}
			default:
				properties[key] = map[string]any{
					"type":        "string",
					"description": "Tool input value.",
				}
			}
			if key != "username" {
				required = append(required, key)
			}
		}
		parameters := map[string]any{
			"type":                 "object",
			"properties":           properties,
			"additionalProperties": true,
		}
		if len(required) > 0 {
			parameters["required"] = required
		}
		result = append(result, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  parameters,
			},
		})
	}
	return result
}

func (a *App) executeChildToolCall(ctx context.Context, username string, iteration int, call llm.ToolCall, allowedTools []tools.Tool, childSessionID int64) (multiagent.TraceStep, string, llm.Message, error) {
	input, err := executeToolInput(call.Function.Arguments)
	if err != nil {
		return multiagent.TraceStep{
			Iteration: iteration,
			Type:      "tool",
			Tool:      call.Function.Name,
			Error:     "invalid tool arguments: " + err.Error(),
		}, "Child produced invalid tool arguments for " + call.Function.Name, llm.Message{}, err
	}
	action := childAction{
		Type:  "tool",
		Tool:  call.Function.Name,
		Input: input,
	}
	step, risk, toolMsg, execErr := a.executeChildAction(ctx, username, iteration, action, allowedTools, childSessionID)
	toolMsg.ToolCallID = call.ID
	toolMsg.Name = call.Function.Name
	return step, risk, toolMsg, execErr
}

func (a *App) executeChildAction(ctx context.Context, username string, iteration int, action childAction, allowedTools []tools.Tool, childSessionID int64) (multiagent.TraceStep, string, llm.Message, error) {
	step := multiagent.TraceStep{
		Iteration: iteration,
		Type:      "tool",
		Tool:      action.Tool,
		Input:     mapsClone(action.Input),
	}
	if !toolAllowed(action.Tool, allowedTools) {
		step.Error = "tool not allowed"
		_ = retryBusy(func() error {
			return a.Store.AddMessage(ctx, childSessionID, "tool", fmt.Sprintf("%s error: %s", action.Tool, step.Error))
		})
		return step, "Child attempted a tool outside its allowlist: " + action.Tool, llm.Message{}, fmt.Errorf("%s", step.Error)
	}
	input := mapsClone(action.Input)
	input["username"] = username
	output, execErr := a.Tools.Execute(ctx, action.Tool, input)
	if execErr != nil {
		step.Error = execErr.Error()
		_ = retryBusy(func() error {
			return a.Store.AddMessage(ctx, childSessionID, "tool", fmt.Sprintf("%s error: %s", action.Tool, execErr.Error()))
		})
		return step, "Child tool execution failed: " + action.Tool, llm.Message{}, execErr
	}
	step.Output = output
	step.Verified, step.Verifier, step.VerificationClass = verifyDelegatedToolOutput(action.Tool, output)
	rawOut, _ := json.Marshal(output)
	content := string(rawOut)
	_ = retryBusy(func() error {
		return a.Store.AddMessage(ctx, childSessionID, "tool", fmt.Sprintf("%s result: %s", action.Tool, content))
	})
	return step, "", llm.Message{Role: "tool", Content: content, Name: action.Tool}, nil
}

func summarizeMultiAgentAggregate(objective string, aggregate multiagent.Aggregate) string {
	lines := []string{
		fmt.Sprintf("[multiagent aggregate] objective=%s", objective),
		fmt.Sprintf("completed=%d failed=%d skipped=%d", aggregate.Completed, aggregate.Failed, aggregate.Skipped),
	}
	if len(aggregate.Risks) > 0 {
		lines = append(lines, "risks="+strings.Join(aggregate.Risks, " | "))
	}
	if len(aggregate.NextActions) > 0 {
		lines = append(lines, "next_actions="+strings.Join(aggregate.NextActions, " | "))
	}
	return strings.Join(lines, "\n")
}

func rebuildTaskFromChildSession(ctx context.Context, st *store.Store, session store.Session, allowedTools []string, historyWindow int) (multiagent.Task, error) {
	prompt := strings.TrimSpace(session.Prompt)
	if prompt == "" {
		return multiagent.Task{}, fmt.Errorf("child session prompt is empty")
	}
	task := multiagent.Task{
		ID:            session.TaskID,
		Title:         session.TaskID,
		Goal:          prompt,
		AllowedTools:  allowedTools,
		HistoryWindow: historyWindow,
	}
	if task.ID == "" {
		task.ID = fmt.Sprintf("resume-%d", session.ID)
	}
	if historyWindow <= 0 {
		task.HistoryWindow = 6
	}
	lines := strings.Split(prompt, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Task ID:"):
			if value := strings.TrimSpace(strings.TrimPrefix(line, "Task ID:")); value != "" {
				task.ID = value
			}
		case strings.HasPrefix(line, "Title:"):
			if value := strings.TrimSpace(strings.TrimPrefix(line, "Title:")); value != "" {
				task.Title = value
			}
		case strings.HasPrefix(line, "Goal:"):
			if value := strings.TrimSpace(strings.TrimPrefix(line, "Goal:")); value != "" {
				task.Goal = value
			}
		}
	}
	if len(task.AllowedTools) == 0 {
		traceRows, err := st.ListMultiAgentTraces(ctx, store.MultiAgentTraceFilters{
			Username:       session.Username,
			ChildSessionID: session.ID,
			Limit:          100,
		})
		if err != nil {
			return multiagent.Task{}, err
		}
		seen := map[string]struct{}{}
		for _, row := range traceRows {
			if strings.TrimSpace(row.Tool) == "" {
				continue
			}
			if _, ok := seen[row.Tool]; ok {
				continue
			}
			seen[row.Tool] = struct{}{}
			task.AllowedTools = append(task.AllowedTools, row.Tool)
		}
	}
	if len(task.AllowedTools) == 0 {
		task.AllowedTools = []string{"session.history", "session.search", "memory.read"}
	}
	return task, nil
}

func deriveResumeBasis(traceRows []store.MultiAgentTraceRecord, messages []store.Message) ResumeBasis {
	var basis ResumeBasis
	for _, row := range traceRows {
		if row.Iteration > basis.LastIteration {
			basis.LastIteration = row.Iteration
		}
		if row.Tool != "" && row.Error == "" {
			basis.LastSuccessfulTool = row.Tool
			basis.LastSuccessfulOut = row.OutputJSON
		}
		if row.Error != "" {
			basis.LastFailedTool = row.Tool
			basis.LastFailedError = row.Error
			if row.InputJSON != "" {
				var input map[string]any
				if err := json.Unmarshal([]byte(row.InputJSON), &input); err == nil {
					basis.LastFailedInput = input
				}
			}
		}
		if row.SnapshotJSON != "" {
			var snapshot map[string]any
			if err := json.Unmarshal([]byte(row.SnapshotJSON), &snapshot); err == nil {
				basis.LastSnapshot = snapshot
				if prompt, ok := snapshot["current_prompt"].(string); ok {
					basis.LastSnapshotPrompt = strings.TrimSpace(prompt)
				}
				switch raw := snapshot["history_len"].(type) {
				case float64:
					basis.LastSnapshotHistoryLen = int(raw)
				case int:
					basis.LastSnapshotHistoryLen = raw
				}
				switch raw := snapshot["next_iteration"].(type) {
				case float64:
					basis.LastSnapshotNextIteration = int(raw)
				case int:
					basis.LastSnapshotNextIteration = raw
				}
				if runtime, ok := snapshot["runtime"].(string); ok {
					basis.LastSnapshotRuntime = strings.TrimSpace(runtime)
				}
				if rawHistory, ok := snapshot["history"].([]any); ok {
					basis.LastSnapshotHistory = decodeSnapshotMessages(rawHistory)
				}
				if rawRisks, ok := snapshot["tool_risks"].([]any); ok {
					basis.LastSnapshotToolRisks = decodeStringSlice(rawRisks)
				}
			}
		}
	}
	basis.RecoveredToolStates = collectRecoveredToolStates(traceRows, 3)
	basis.RecoveredHistoryMessage = len(buildResumeHistoryMessages(messages, 8))
	return basis
}

func buildResumeContext(childSessionID int64, basis ResumeBasis) string {
	lines := []string{
		fmt.Sprintf("Recovery mode: continue from prior child session %d.", childSessionID),
		fmt.Sprintf("Resume from iteration %d.", basis.LastIteration),
	}
	if basis.RecoveredHistoryMessage > 0 {
		lines = append(lines, fmt.Sprintf("Recovered %d prior assistant/tool history messages.", basis.RecoveredHistoryMessage))
	}
	if len(basis.LastSnapshot) > 0 {
		raw, _ := json.Marshal(basis.LastSnapshot)
		lines = append(lines, "Recovered loop snapshot: "+string(raw))
	}
	if basis.LastSnapshotPrompt != "" {
		lines = append(lines, "Recovered next prompt candidate: "+basis.LastSnapshotPrompt)
	}
	if basis.LastSnapshotHistoryLen > 0 {
		lines = append(lines, fmt.Sprintf("Recovered snapshot history length: %d", basis.LastSnapshotHistoryLen))
	}
	if len(basis.LastSnapshotHistory) > 0 {
		lines = append(lines, fmt.Sprintf("Recovered exact snapshot history messages: %d", len(basis.LastSnapshotHistory)))
	}
	if basis.LastSnapshotNextIteration > 0 {
		lines = append(lines, fmt.Sprintf("Recovered next iteration: %d", basis.LastSnapshotNextIteration))
	}
	if basis.LastSnapshotRuntime != "" {
		lines = append(lines, "Recovered runtime mode: "+basis.LastSnapshotRuntime)
	}
	if len(basis.LastSnapshotToolRisks) > 0 {
		lines = append(lines, "Recovered tool risks: "+strings.Join(basis.LastSnapshotToolRisks, "; "))
	}
	if basis.LastSuccessfulTool != "" {
		lines = append(lines, "Last successful tool: "+basis.LastSuccessfulTool)
		if basis.LastSuccessfulOut != "" {
			lines = append(lines, "Last successful output: "+basis.LastSuccessfulOut)
		}
	}
	if len(basis.RecoveredToolStates) > 0 {
		summaries := make([]string, 0, len(basis.RecoveredToolStates))
		for _, state := range basis.RecoveredToolStates {
			summaries = append(summaries, fmt.Sprintf("%s@%d", state.Tool, state.Iteration))
		}
		lines = append(lines, "Recovered tool states: "+strings.Join(summaries, ", "))
	}
	if basis.LastFailedTool != "" {
		lines = append(lines, "Last failed tool: "+basis.LastFailedTool)
		lines = append(lines, "Last failure: "+basis.LastFailedError)
		if len(basis.LastFailedInput) > 0 {
			raw, _ := json.Marshal(basis.LastFailedInput)
			lines = append(lines, "Last failed input: "+string(raw))
		}
	}
	return strings.Join(lines, "\n")
}

func buildResumeSeedHistory(messages []store.Message, basis ResumeBasis) []llm.Message {
	if len(basis.LastSnapshotHistory) > 0 {
		seedHistory := append([]llm.Message(nil), basis.LastSnapshotHistory...)
		for _, state := range basis.RecoveredToolStates {
			if state.Output == "" {
				continue
			}
			seedHistory = append(seedHistory, llm.Message{
				Role:    "tool",
				Name:    state.Tool,
				Content: state.Output,
			})
		}
		return seedHistory
	}
	seedHistory := buildResumeHistoryMessages(messages, 8)
	if len(seedHistory) > 0 {
		return seedHistory
	}
	if basis.LastSuccessfulTool == "" || strings.TrimSpace(basis.LastSuccessfulOut) == "" {
		return nil
	}
	return []llm.Message{{
		Role:    "tool",
		Name:    basis.LastSuccessfulTool,
		Content: basis.LastSuccessfulOut,
	}}
}

func buildResumeLoopSeed(basis ResumeBasis) childLoopSeed {
	nextIteration := basis.LastSnapshotNextIteration
	if nextIteration <= 0 {
		nextIteration = basis.LastIteration + 1
	}
	return childLoopSeed{
		CurrentPrompt: strings.TrimSpace(basis.LastSnapshotPrompt),
		NextIteration: nextIteration,
		Runtime:       strings.TrimSpace(basis.LastSnapshotRuntime),
		ToolRisks:     append([]string(nil), basis.LastSnapshotToolRisks...),
	}
}

func collectRecoveredToolStates(traceRows []store.MultiAgentTraceRecord, limit int) []ResumeToolState {
	if limit <= 0 {
		limit = 3
	}
	states := make([]ResumeToolState, 0, limit)
	for i := len(traceRows) - 1; i >= 0 && len(states) < limit; i-- {
		row := traceRows[i]
		if row.Tool == "" || row.Error != "" || strings.TrimSpace(row.OutputJSON) == "" {
			continue
		}
		states = append(states, ResumeToolState{
			Iteration: row.Iteration,
			Tool:      row.Tool,
			Output:    row.OutputJSON,
		})
	}
	for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
		states[i], states[j] = states[j], states[i]
	}
	return states
}

func buildResumeHistoryMessages(messages []store.Message, limit int) []llm.Message {
	if limit <= 0 {
		limit = 8
	}
	filtered := make([]store.Message, 0, limit)
	for i := len(messages) - 1; i >= 0 && len(filtered) < limit; i-- {
		msg := messages[i]
		if msg.Role != "assistant" && msg.Role != "tool" {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		filtered = append(filtered, msg)
	}
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	result := make([]llm.Message, 0, len(filtered))
	for _, msg := range filtered {
		switch msg.Role {
		case "assistant":
			result = append(result, llm.NewMessage("assistant", msg.Content))
		case "tool":
			name, content := parseStoredToolMessage(msg.Content)
			toolMsg := llm.Message{
				Role:    "tool",
				Content: content,
				Name:    name,
			}
			result = append(result, toolMsg)
		}
	}
	return result
}

func decodeStringSlice(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if str, ok := value.(string); ok && strings.TrimSpace(str) != "" {
			result = append(result, strings.TrimSpace(str))
		}
	}
	return result
}

func parseStoredToolMessage(content string) (string, string) {
	for _, marker := range []string{" result: ", " error: "} {
		if idx := strings.Index(content, marker); idx > 0 {
			return strings.TrimSpace(content[:idx]), strings.TrimSpace(content[idx+len(marker):])
		}
	}
	return "", strings.TrimSpace(content)
}

func (a *App) childLoopSnapshot(iteration int, task multiagent.Task, currentPrompt string, history []llm.Message, allowedTools []tools.Tool, runtime string, toolRisks []string) multiagent.TraceStep {
	toolNames := make([]string, 0, len(allowedTools))
	for _, tool := range allowedTools {
		toolNames = append(toolNames, tool.Name)
	}
	historySnapshot := make([]map[string]any, 0, len(history))
	for _, msg := range history {
		entry := map[string]any{
			"role":    msg.Role,
			"content": strings.TrimSpace(msg.Content),
		}
		if strings.TrimSpace(msg.Name) != "" {
			entry["name"] = strings.TrimSpace(msg.Name)
		}
		if strings.TrimSpace(msg.ToolCallID) != "" {
			entry["tool_call_id"] = strings.TrimSpace(msg.ToolCallID)
		}
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				toolCalls = append(toolCalls, map[string]any{
					"id":   call.ID,
					"type": call.Type,
					"function": map[string]any{
						"name":      call.Function.Name,
						"arguments": call.Function.Arguments,
					},
				})
			}
			entry["tool_calls"] = toolCalls
		}
		historySnapshot = append(historySnapshot, entry)
	}
	return multiagent.TraceStep{
		Iteration: iteration,
		Type:      "snapshot",
		Snapshot: map[string]any{
			"task_id":        task.ID,
			"current_prompt": strings.TrimSpace(currentPrompt),
			"history_len":    len(history),
			"history":        historySnapshot,
			"allowed_tools":  toolNames,
			"next_iteration": iteration + 1,
			"runtime":        strings.TrimSpace(runtime),
			"tool_risks":     append([]string(nil), toolRisks...),
		},
		Note: "child loop snapshot",
	}
}

func verifyDelegatedToolOutput(toolName string, output map[string]any) (bool, string, string) {
	switch toolName {
	case "session.search":
		_, ok := output["results"]
		if ok {
			return true, "results field check", "ok"
		}
		return false, "results field check", "missing_results_field"
	case "session.history":
		_, ok := output["sessions"]
		if ok {
			return true, "sessions field check", "ok"
		}
		return false, "sessions field check", "missing_sessions_field"
	case "memory.read":
		_, ok := output["memory"]
		if ok {
			return true, "memory field check", "ok"
		}
		return false, "memory field check", "missing_memory_field"
	case "system.exec_profile":
		_, ok := output["results"]
		if ok {
			return true, "execution profile results check", "ok"
		}
		return false, "execution profile results check", "missing_results_field"
	default:
		if len(output) > 0 {
			return true, "non-empty output check", "ok"
		}
		return false, "non-empty output check", "empty_output"
	}
}

func decodeSnapshotMessages(rawHistory []any) []llm.Message {
	result := make([]llm.Message, 0, len(rawHistory))
	for _, raw := range rawHistory {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		msg := llm.Message{}
		msg.Role, _ = entry["role"].(string)
		msg.Content, _ = entry["content"].(string)
		msg.Name, _ = entry["name"].(string)
		msg.ToolCallID, _ = entry["tool_call_id"].(string)
		if rawCalls, ok := entry["tool_calls"].([]any); ok {
			msg.ToolCalls = make([]llm.ToolCall, 0, len(rawCalls))
			for _, rawCall := range rawCalls {
				callMap, ok := rawCall.(map[string]any)
				if !ok {
					continue
				}
				call := llm.ToolCall{}
				call.ID, _ = callMap["id"].(string)
				call.Type, _ = callMap["type"].(string)
				if fn, ok := callMap["function"].(map[string]any); ok {
					call.Function.Name, _ = fn["name"].(string)
					call.Function.Arguments, _ = fn["arguments"].(string)
				}
				msg.ToolCalls = append(msg.ToolCalls, call)
			}
		}
		result = append(result, msg)
	}
	return result
}

func retryBusy(fn func() error) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "locked") && !strings.Contains(strings.ToLower(err.Error()), "busy") {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
	}
	return err
}

type childAction struct {
	Type        string         `json:"type"`
	Tool        string         `json:"tool,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
	Note        string         `json:"note,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Risks       []string       `json:"risks,omitempty"`
	NextActions []string       `json:"next_actions,omitempty"`
}

func parseChildAction(raw string) (childAction, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var action childAction
	if err := json.Unmarshal([]byte(raw), &action); err != nil {
		return childAction{}, err
	}
	return action, nil
}

func toolAllowed(name string, allowed []tools.Tool) bool {
	for _, tool := range allowed {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func mapsClone(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
