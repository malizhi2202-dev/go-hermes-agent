package app

import (
	"context"
	"fmt"
	"os"
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
	MultiAgent *multiagent.Orchestrator
}

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
		),
	}
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
		summary, err := application.LLM.ChatWithContext(ctx, []string{
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

func (a *App) Close() error {
	return a.Store.Close()
}

func (a *App) Chat(ctx context.Context, username, prompt string) (string, error) {
	a.mu.RLock()
	client := a.LLM
	model := a.Config.ResolvedLLM().Model
	mem := a.Memory
	compressor := a.Compressor
	historyWindow := a.Config.Context.HistoryWindowMessages
	maxPromptChars := a.Config.Context.MaxPromptChars
	a.mu.RUnlock()
	memoryContext, err := mem.Prefetch(ctx, username, prompt)
	if err != nil {
		return "", err
	}
	systemBlocks := []string(nil)
	if memoryContext != "" {
		systemBlocks = append(systemBlocks, memoryContext)
	}
	storedSummary, err := a.Store.GetContextSummary(ctx, username)
	if err != nil {
		return "", err
	}
	if storedSummary.Summary != "" {
		systemBlocks = append(systemBlocks, storedSummary.Summary)
	}
	recentMessages, err := a.Store.ListRecentMessagesByUsername(ctx, username, historyWindow)
	if err != nil {
		return "", err
	}
	history := make([]llmMessage, 0, len(recentMessages))
	for _, msg := range recentMessages {
		history = append(history, llmMessage{Role: msg.Role, Content: msg.Content})
	}
	compression := compressor.Compress(ctx, storedSummary.Summary, convertHistory(history))
	if compression.Compressed && compression.SystemBlock != "" {
		if storedSummary.Summary == "" {
			systemBlocks = append(systemBlocks, compression.SystemBlock)
		} else {
			systemBlocks[len(systemBlocks)-1] = compression.SystemBlock
		}
		history = convertBackHistory(compression.History)
		if err := a.Store.UpsertContextSummary(ctx, username, compression.PersistedSummary, a.Config.Context.SummaryStrategy); err != nil {
			return "", err
		}
		_ = a.Store.WriteAudit(ctx, username, "context_compress_applied", fmt.Sprintf("compressed_messages=%d", compression.CompressedMessages))
	}
	trimHistoryToBudget(systemBlocks, &history, prompt, maxPromptChars)
	response, err := client.ChatWithMessages(ctx, systemBlocks, convertHistory(history), prompt)
	if err != nil {
		return "", err
	}
	sessionID, err := a.Store.CreateSession(ctx, username, model, prompt, response)
	if err != nil {
		return "", err
	}
	if err := a.Store.AddMessage(ctx, sessionID, "user", prompt); err != nil {
		return "", err
	}
	if err := a.Store.AddMessage(ctx, sessionID, "assistant", response); err != nil {
		return "", err
	}
	if err := mem.SyncTurn(ctx, username, prompt, response); err != nil {
		return "", err
	}
	_ = a.Store.WriteAudit(ctx, username, "chat", "session recorded")
	return response, nil
}

type llmMessage struct {
	Role    string
	Content string
}

func convertHistory(history []llmMessage) []llm.Message {
	result := make([]llm.Message, 0, len(history))
	for _, item := range history {
		result = append(result, llm.NewMessage(item.Role, item.Content))
	}
	return result
}

func convertBackHistory(history []llm.Message) []llmMessage {
	result := make([]llmMessage, 0, len(history))
	for _, item := range history {
		result = append(result, llmMessage{Role: item.Role, Content: item.Content})
	}
	return result
}

func trimHistoryToBudget(systemBlocks []string, history *[]llmMessage, prompt string, maxChars int) {
	if maxChars <= 0 {
		return
	}
	total := len(prompt) + len("You are a secure, concise assistant.")
	for _, block := range systemBlocks {
		total += len(block)
	}
	for _, item := range *history {
		total += len(item.Content)
	}
	for total > maxChars && len(*history) > 0 {
		total -= len((*history)[0].Content)
		*history = (*history)[1:]
	}
}

func (a *App) EstimateContextBudget(ctx context.Context, username, prompt string) (ContextBudget, error) {
	a.mu.RLock()
	model := a.Config.ResolvedLLM().Model
	historyWindow := a.Config.Context.HistoryWindowMessages
	maxPromptChars := a.Config.Context.MaxPromptChars
	mem := a.Memory
	compressor := a.Compressor
	a.mu.RUnlock()
	memoryContext, err := mem.Prefetch(ctx, username, prompt)
	if err != nil {
		return ContextBudget{}, err
	}
	systemBlockContents := make([]string, 0, 2)
	promptChars := len(prompt) + len("You are a secure, concise assistant.")
	if memoryContext != "" {
		systemBlockContents = append(systemBlockContents, memoryContext)
		promptChars += len(memoryContext)
	}
	storedSummary, err := a.Store.GetContextSummary(ctx, username)
	if err != nil {
		return ContextBudget{}, err
	}
	if storedSummary.Summary != "" {
		systemBlockContents = append(systemBlockContents, storedSummary.Summary)
		promptChars += len(storedSummary.Summary)
	}
	recentMessages, err := a.Store.ListRecentMessagesByUsername(ctx, username, historyWindow)
	if err != nil {
		return ContextBudget{}, err
	}
	history := make([]llmMessage, 0, len(recentMessages))
	for _, msg := range recentMessages {
		history = append(history, llmMessage{Role: msg.Role, Content: msg.Content})
	}
	compression := compressor.Compress(ctx, storedSummary.Summary, convertHistory(history))
	if compression.Compressed && compression.SystemBlock != "" {
		if storedSummary.Summary == "" {
			systemBlockContents = append(systemBlockContents, compression.SystemBlock)
			promptChars += len(compression.SystemBlock)
		} else {
			promptChars -= len(storedSummary.Summary)
			systemBlockContents[len(systemBlockContents)-1] = compression.SystemBlock
			promptChars += len(compression.SystemBlock)
		}
		history = convertBackHistory(compression.History)
	}
	trimHistoryToBudget(systemBlockContents, &history, prompt, maxPromptChars)
	for _, item := range history {
		promptChars += len(item.Content)
	}
	return ContextBudget{
		Model:                   model,
		HistoryWindowMessages:   historyWindow,
		HistoryMessagesUsed:     len(history),
		CompressionEnabled:      a.Config.Context.CompressionEnabled,
		SummaryStrategy:         a.Config.Context.SummaryStrategy,
		Compressed:              compression.Compressed,
		CompressedMessages:      compression.CompressedMessages,
		CompressionSummaryChars: compression.SummaryChars,
		PersistedSummaryChars:   len(storedSummary.Summary),
		TailMessagesUsed:        compression.TailMessagesUsed,
		SystemBlocksUsed:        len(systemBlockContents),
		PromptChars:             promptChars,
		MaxPromptChars:          maxPromptChars,
	}, nil
}

func (a *App) CurrentLLM() config.LLMConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Config.ResolvedLLM()
}

func (a *App) CurrentModelProfile() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Config.CurrentModelProfile
}

func (a *App) ListModelProfiles() map[string]config.LLMConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Config.ListModelProfiles()
}

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

func (a *App) ResolveModelProfile(name string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return models.DefaultCatalog().ResolveProfile(a.Config.ListModelProfiles(), name)
}

func (a *App) DiscoverLocalModels(ctx context.Context) ([]models.DiscoveredModel, error) {
	a.mu.RLock()
	profiles := a.Config.ListModelProfiles()
	a.mu.RUnlock()
	return models.DiscoverLocalModels(ctx, profiles)
}

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

func (a *App) BuildMultiAgentPlan(ctx context.Context, username, objective string, tasks []multiagent.Task) (multiagent.Plan, error) {
	plan, err := a.MultiAgent.BuildPlan(objective, tasks)
	if err != nil {
		return multiagent.Plan{}, err
	}
	_ = a.Store.WriteAudit(ctx, username, "multiagent_plan_built", fmt.Sprintf("tasks=%d mode=%s", len(plan.Tasks), plan.Mode))
	return plan, nil
}

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

func (a *App) runMultiAgentChildTask(ctx context.Context, username string, parentSessionID int64, task multiagent.Task) multiagent.Result {
	llmCfg := a.CurrentLLM()
	allowedTools, invalidTools := a.resolveChildAllowedTools(task.AllowedTools)
	prompt, historySnippet := a.buildMultiAgentTaskPrompt(ctx, parentSessionID, task)
	summary, runtime, err := a.tryRunMultiAgentLLMTask(ctx, llmCfg, task, prompt, historySnippet, allowedTools)
	risks := make([]string, 0, 2)
	if len(invalidTools) > 0 {
		risks = append(risks, "Some requested child tools are not available: "+strings.Join(invalidTools, ", "))
	}
	if err == nil {
		childSessionID, sessionErr := a.recordMultiAgentChildSession(ctx, username, parentSessionID, task, prompt, fmt.Sprintf("[runtime=%s] %s", runtime, summary))
		if sessionErr != nil {
			return multiagent.Result{
				TaskID:      task.ID,
				Status:      multiagent.ResultFailed,
				Summary:     sessionErr.Error(),
				Risks:       []string{"Failed to persist child session."},
				NextActions: []string{"Check SQLite session persistence path."},
			}
		}
		return multiagent.Result{
			TaskID:         task.ID,
			ChildSessionID: childSessionID,
			Status:         multiagent.ResultCompleted,
			Summary:        fmt.Sprintf("[runtime=%s] %s", runtime, summary),
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
	childSessionID, sessionErr := a.recordMultiAgentChildSession(ctx, username, parentSessionID, task, prompt, "[runtime=stub] "+fallback)
	if sessionErr != nil {
		return multiagent.Result{
			TaskID:      task.ID,
			Status:      multiagent.ResultFailed,
			Summary:     sessionErr.Error(),
			Risks:       []string{"Failed to persist child session."},
			NextActions: []string{"Check SQLite session persistence path."},
		}
	}
	return multiagent.Result{
		TaskID:         task.ID,
		ChildSessionID: childSessionID,
		Status:         multiagent.ResultCompleted,
		Summary:        "[runtime=stub] " + fallback,
		Risks:          append(risks, "Child task runtime fell back to stub execution."),
		NextActions:    []string{"Provide API key or local model endpoint to enable LLM child runtime.", "Attach delegated tool runtime for real task execution."},
	}
}

func (a *App) tryRunMultiAgentLLMTask(ctx context.Context, llmCfg config.LLMConfig, task multiagent.Task, prompt, historySnippet string, allowedTools []tools.Tool) (string, string, error) {
	if strings.TrimSpace(llmCfg.BaseURL) == "" || strings.TrimSpace(llmCfg.Model) == "" {
		return "", "", fmt.Errorf("llm is not configured")
	}
	if strings.TrimSpace(llmCfg.APIKeyEnv) != "" && strings.TrimSpace(os.Getenv(llmCfg.APIKeyEnv)) == "" {
		return "", "", fmt.Errorf("missing llm api key")
	}
	systemBlocks := []string{
		"You are a focused child agent working on one delegated subtask.",
		"Do not delegate further. Do not invent tool execution. Summarize what should be done or what you found using only the provided task context.",
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
	response, err := a.LLM.ChatWithContext(ctx, systemBlocks, prompt)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(response), "llm", nil
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
	if err := retryBusy(func() error {
		return a.Store.AddMessage(ctx, childSessionID, "assistant", summary)
	}); err != nil {
		return 0, err
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
