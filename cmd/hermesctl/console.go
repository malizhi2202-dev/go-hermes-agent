package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"go-hermes-agent/internal/app"
	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/cron"
	"go-hermes-agent/internal/store"
	"go-hermes-agent/internal/trajectory"
)

// InteractiveConsole is the unified CLI control plane used by `hermesctl chat`
// when running in interactive mode.
//
// Design notes:
//   - plain text lines continue to behave like chat turns
//   - slash commands expose the most-used admin and observability workflows
//   - login state is kept in memory so users do not need to retype full shell commands
type InteractiveConsole struct {
	configPath           string
	app                  *app.App
	cfg                  config.Config
	reader               *bufio.Reader
	out                  io.Writer
	user                 string
	turnCount            int
	turns                []chatTurn
	lastChat             app.ChatResult
	activeSessionID      int64
	activeSessionManaged bool
}

type chatTurn struct {
	SessionID int64
	Prompt    string
	Response  string
	Model     string
	Managed   bool
}

func newInteractiveConsole(configPath string, cfg config.Config, application *app.App, reader *bufio.Reader, out io.Writer, user string) *InteractiveConsole {
	return &InteractiveConsole{
		configPath: configPath,
		app:        application,
		cfg:        cfg,
		reader:     reader,
		out:        out,
		user:       user,
	}
}

func (c *InteractiveConsole) Start(ctx context.Context) {
	current := c.app.CurrentLLM()
	fmt.Fprintf(c.out, "console started as %q with model %q\n", c.user, current.Model)
	fmt.Fprintln(c.out, "type /help for commands, /exit to leave")
	for {
		fmt.Fprint(c.out, "you> ")
		line, err := c.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			fmt.Fprintf(c.out, "assistant> error: %v\n", err)
			return
		}
		text := strings.TrimSpace(line)
		if text == "" {
			if err == io.EOF {
				fmt.Fprintln(c.out)
				return
			}
			continue
		}
		keepGoing := c.handleLine(ctx, text)
		if !keepGoing || err == io.EOF {
			return
		}
	}
}

func (c *InteractiveConsole) handleLine(ctx context.Context, text string) bool {
	switch text {
	case "/exit", "/quit":
		return false
	}
	if !strings.HasPrefix(text, "/") {
		var (
			result  app.ChatResult
			err     error
			managed = true
		)
		if c.activeSessionID > 0 {
			result, err = c.app.ChatInSessionDetailed(ctx, c.user, c.activeSessionID, text)
			managed = c.activeSessionManaged
		} else {
			result, err = c.app.ChatDetailed(ctx, c.user, text)
		}
		if err != nil {
			fmt.Fprintf(c.out, "assistant> error: %v\n", err)
			return true
		}
		c.recordChatTurn(result, managed)
		if c.activeSessionID == 0 && result.SessionID > 0 {
			c.activeSessionID = result.SessionID
			c.activeSessionManaged = true
		}
		fmt.Fprintf(c.out, "assistant> %s\n", result.Response)
		return true
	}

	commandLine := strings.TrimPrefix(text, "/")
	command, rest := splitCommandAndRest(commandLine)
	switch command {
	case "help":
		c.printHelp()
	case "new":
		c.handleNew()
	case "clear":
		c.handleClear()
	case "login":
		c.handleLogin(ctx, rest)
	case "whoami":
		c.writeJSON(map[string]any{"username": c.user, "model": c.app.CurrentLLM().Model, "profile": c.app.CurrentModelProfile()})
	case "status":
		c.handleStatus()
	case "usage":
		c.handleUsage(ctx)
	case "insights":
		c.handleInsights(ctx, rest)
	case "prompt-inspect":
		c.handlePromptInspect(ctx, rest)
	case "prompt-cache-stats":
		c.handlePromptCacheStats()
	case "prompt-cache-clear":
		c.handlePromptCacheClear()
	case "prompt-config":
		c.handlePromptConfig()
	case "models":
		c.handleModels()
	case "discover-models":
		c.handleDiscoverModels(ctx)
	case "model":
		c.handleModelSwitch(ctx, rest)
	case "model-metadata":
		c.handleModelMetadata(rest)
	case "auxiliary-info":
		c.handleAuxiliaryInfo(rest)
	case "auxiliary-chat":
		c.handleAuxiliaryChat(ctx, rest)
	case "auxiliary-switch":
		c.handleAuxiliarySwitch(ctx, rest)
	case "resume":
		c.handleResume(ctx, rest)
	case "retry":
		c.handleRetry(ctx)
	case "undo":
		c.handleUndo(ctx)
	case "context":
		c.handleContext(ctx, rest)
	case "sessions":
		c.handleSessions(ctx, rest)
	case "history":
		c.handleHistory(ctx, rest)
	case "search":
		c.handleSearch(ctx, rest)
	case "audit":
		c.handleAudit(ctx, rest)
	case "extensions":
		c.handleExtensions(ctx)
	case "extension-hooks":
		c.handleExtensionHooks(ctx, rest)
	case "extension-refresh":
		c.handleExtensionRefresh(ctx)
	case "extension-state":
		c.handleExtensionState(ctx, rest)
	case "extension-validate":
		c.handleExtensionValidate(ctx, rest)
	case "tools":
		c.writeJSON(c.app.Tools.List())
	case "multiagent-plan":
		c.handleMultiAgentPlan(ctx, rest)
	case "multiagent-run":
		c.handleMultiAgentRun(ctx, rest)
	case "multiagent-traces":
		c.handleMultiAgentTraces(ctx, rest)
	case "multiagent-summary":
		c.handleMultiAgentSummary(ctx, rest)
	case "multiagent-verifiers":
		c.handleMultiAgentVerifiers(ctx, rest)
	case "multiagent-failures":
		c.handleMultiAgentFailures(ctx, rest)
	case "multiagent-hotspots":
		c.handleMultiAgentHotspots(ctx, rest)
	case "multiagent-replay":
		c.handleMultiAgentReplay(ctx, rest)
	case "multiagent-resume":
		c.handleMultiAgentResume(ctx, rest)
	case "trajectories":
		c.handleTrajectories(rest)
	case "trajectory-show":
		c.handleTrajectoryShow(rest)
	case "trajectory-summary":
		c.handleTrajectorySummary(rest)
	case "cron-add":
		c.handleCronAdd(rest)
	case "cron-list":
		c.handleCronList()
	case "cron-show":
		c.handleCronShow(rest)
	case "cron-delete":
		c.handleCronDelete(rest)
	case "cron-tick":
		c.handleCronTick(ctx)
	case "tool-exec":
		c.handleToolExec(ctx, rest)
	case "execution-audit":
		c.handleExecutionAudit(ctx, rest)
	case "execution-profile-audit":
		c.handleExecutionProfileAudit(ctx, rest)
	default:
		fmt.Fprintf(c.out, "assistant> unknown command %q, use /help\n", command)
	}
	return true
}

func (c *InteractiveConsole) recordChatTurn(result app.ChatResult, managed bool) {
	c.lastChat = result
	c.turnCount++
	c.turns = append(c.turns, chatTurn{
		SessionID: result.SessionID,
		Prompt:    result.Prompt,
		Response:  result.Response,
		Model:     result.Model,
		Managed:   managed,
	})
}

func (c *InteractiveConsole) resetTurnState() {
	c.turnCount = 0
	c.turns = nil
	c.lastChat = app.ChatResult{}
	c.activeSessionID = 0
	c.activeSessionManaged = false
}

func (c *InteractiveConsole) trackedSessionCount() int {
	if len(c.turns) == 0 {
		if c.activeSessionID > 0 {
			return 1
		}
		return 0
	}
	seen := map[int64]struct{}{}
	for _, turn := range c.turns {
		if turn.SessionID > 0 {
			seen[turn.SessionID] = struct{}{}
		}
	}
	if c.activeSessionID > 0 {
		seen[c.activeSessionID] = struct{}{}
	}
	return len(seen)
}

func (c *InteractiveConsole) handleNew() {
	c.resetTurnState()
	fmt.Fprintln(c.out, "assistant> started a fresh console workflow state")
}

func (c *InteractiveConsole) handleClear() {
	c.resetTurnState()
	fmt.Fprint(c.out, "\033[H\033[2J")
	fmt.Fprintln(c.out, "assistant> cleared screen and reset console workflow state")
}

func (c *InteractiveConsole) handleLogin(ctx context.Context, rest string) {
	args := strings.Fields(rest)
	username := ""
	password := ""
	if len(args) >= 1 {
		username = strings.TrimSpace(args[0])
	}
	if len(args) >= 2 {
		password = strings.TrimSpace(args[1])
	}
	if username == "" {
		username = c.prompt("username")
	}
	if password == "" {
		password = c.prompt("password")
	}
	user, err := authenticateChatUser(ctx, c.app, username, password, "")
	if err != nil {
		fmt.Fprintf(c.out, "assistant> login failed: %v\n", err)
		return
	}
	c.user = user
	c.resetTurnState()
	fmt.Fprintf(c.out, "assistant> logged in as %s\n", c.user)
}

func (c *InteractiveConsole) handleStatus() {
	payload := map[string]any{
		"username":         c.user,
		"model":            c.app.CurrentLLM().Model,
		"profile":          c.app.CurrentModelProfile(),
		"console_turns":    c.turnCount,
		"tracked_sessions": c.trackedSessionCount(),
	}
	if c.activeSessionID > 0 {
		payload["active_session_id"] = c.activeSessionID
		payload["active_session_managed"] = c.activeSessionManaged
	}
	if c.lastChat.SessionID > 0 {
		payload["last_session_id"] = c.lastChat.SessionID
		payload["last_prompt"] = c.lastChat.Prompt
		payload["last_response"] = c.lastChat.Response
		payload["last_model"] = c.lastChat.Model
	}
	if len(c.turns) > 0 {
		recent := make([]int64, 0, len(c.turns))
		for _, turn := range c.turns {
			recent = append(recent, turn.SessionID)
		}
		payload["recent_session_ids"] = recent
	}
	c.writeJSON(payload)
}

func (c *InteractiveConsole) handleUsage(ctx context.Context) {
	cacheStats := c.app.PromptCacheStats()
	recentSessions, err := c.app.Store.ListSessions(ctx, c.user, 10)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> usage failed: %v\n", err)
		return
	}
	recentMessages, err := c.app.Store.ListRecentMessagesByUsername(ctx, c.user, 20)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> usage failed: %v\n", err)
		return
	}
	payload := map[string]any{
		"username":             c.user,
		"model":                c.app.CurrentLLM().Model,
		"profile":              c.app.CurrentModelProfile(),
		"prompt_cache":         cacheStats,
		"console_turns":        c.turnCount,
		"tracked_sessions":     c.trackedSessionCount(),
		"recent_sessions_seen": len(recentSessions),
		"recent_messages_seen": len(recentMessages),
	}
	if c.lastChat.SessionID > 0 {
		payload["last_session_id"] = c.lastChat.SessionID
	}
	c.writeJSON(payload)
}

func (c *InteractiveConsole) handleInsights(ctx context.Context, rest string) {
	days := parsePositiveIntDefault(strings.TrimSpace(rest), 30)
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	insights, err := c.app.Store.BuildUserInsights(ctx, c.user, since)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> insights failed: %v\n", err)
		return
	}
	payload := map[string]any{
		"days":     days,
		"insights": insights,
	}
	if c.lastChat.SessionID > 0 {
		payload["last_session_id"] = c.lastChat.SessionID
	}
	c.writeJSON(payload)
}

func (c *InteractiveConsole) handlePromptInspect(ctx context.Context, rest string) {
	prompt := strings.TrimSpace(rest)
	if prompt == "" {
		fmt.Fprintln(c.out, "assistant> usage: /prompt-inspect <prompt>")
		return
	}
	plan, err := c.app.InspectPrompt(ctx, c.user, prompt)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> prompt inspect failed: %v\n", err)
		return
	}
	c.writeJSON(plan)
}

func (c *InteractiveConsole) handlePromptCacheStats() {
	c.writeJSON(c.app.PromptCacheStats())
}

func (c *InteractiveConsole) handlePromptCacheClear() {
	c.app.ClearPromptCache()
	c.writeJSON(map[string]any{
		"ok":    true,
		"stats": c.app.PromptCacheStats(),
	})
}

func (c *InteractiveConsole) handlePromptConfig() {
	c.writeJSON(map[string]any{
		"current_profile": c.app.CurrentModelProfile(),
		"current_model":   c.app.CurrentLLM(),
		"context":         c.cfg.Context,
		"prompting":       c.cfg.Prompting,
		"auxiliary":       c.cfg.Auxiliary,
	})
}

func (c *InteractiveConsole) handleModels() {
	c.writeJSON(map[string]any{
		"current_profile": c.app.CurrentModelProfile(),
		"current":         c.app.CurrentLLM(),
		"profiles":        c.app.ListModelProfiles(),
	})
}

func (c *InteractiveConsole) handleDiscoverModels(ctx context.Context) {
	discovered, err := c.app.DiscoverLocalModels(ctx)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> discover models failed: %v\n", err)
		return
	}
	c.writeJSON(map[string]any{
		"count":             len(discovered),
		"discovered_models": discovered,
	})
}

func (c *InteractiveConsole) handleModelSwitch(ctx context.Context, rest string) {
	name := strings.TrimSpace(rest)
	if name == "" {
		fmt.Fprintln(c.out, "assistant> usage: /model <profile-or-alias>")
		return
	}
	if resolved, ok := c.app.ResolveModelProfile(name); ok {
		name = resolved
	}
	if err := c.app.SwitchModelProfile(ctx, c.user, name); err != nil {
		fmt.Fprintf(c.out, "assistant> model switch failed: %v\n", err)
		return
	}
	c.cfg = c.app.Config
	if err := config.Save(c.configPath, c.cfg); err != nil {
		fmt.Fprintf(c.out, "assistant> model switched but config save failed: %v\n", err)
		return
	}
	c.writeJSON(map[string]any{
		"current_profile": c.app.CurrentModelProfile(),
		"current":         c.app.CurrentLLM(),
	})
}

func (c *InteractiveConsole) handleModelMetadata(rest string) {
	name := strings.TrimSpace(rest)
	if name == "" {
		c.writeJSON(map[string]any{
			"current":  c.app.CurrentModelMetadata(),
			"profiles": c.app.ListModelMetadata(),
		})
		return
	}
	metadata := c.app.ListModelMetadata()
	if item, ok := metadata[name]; ok {
		c.writeJSON(map[string]any{
			"profile":  name,
			"metadata": item,
		})
		return
	}
	if resolved, ok := c.app.ResolveModelProfile(name); ok {
		if item, ok := metadata[resolved]; ok {
			c.writeJSON(map[string]any{
				"profile":  resolved,
				"metadata": item,
			})
			return
		}
	}
	fmt.Fprintf(c.out, "assistant> unknown model profile %q\n", name)
}

func (c *InteractiveConsole) handleAuxiliaryInfo(rest string) {
	task := strings.TrimSpace(rest)
	if task == "" {
		task = "general"
	}
	info, err := c.app.AuxiliaryInfo(task)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> auxiliary info failed: %v\n", err)
		return
	}
	c.writeJSON(info)
}

func (c *InteractiveConsole) handleAuxiliaryChat(ctx context.Context, rest string) {
	task := "general"
	prompt := strings.TrimSpace(rest)
	if left, right, ok := splitAroundDoubleDash(rest); ok {
		if strings.TrimSpace(left) != "" {
			task = strings.TrimSpace(left)
		}
		prompt = strings.TrimSpace(right)
	}
	if prompt == "" {
		fmt.Fprintln(c.out, "assistant> usage: /auxiliary-chat [task] -- <prompt>")
		return
	}
	response, info, err := c.app.AuxiliaryChat(ctx, task, prompt)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> auxiliary chat failed: %v\n", err)
		return
	}
	c.writeJSON(map[string]any{
		"task":       task,
		"resolution": info,
		"response":   response,
	})
}

func (c *InteractiveConsole) handleAuxiliarySwitch(ctx context.Context, rest string) {
	args := strings.Fields(rest)
	if len(args) == 0 {
		fmt.Fprintln(c.out, "assistant> usage: /auxiliary-switch <profile> [default|summary|compression]")
		return
	}
	profile := strings.TrimSpace(args[0])
	task := "default"
	if len(args) >= 2 {
		task = strings.TrimSpace(args[1])
	}
	auxCfg, err := c.app.SwitchAuxiliaryProfile(ctx, c.user, task, profile)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> auxiliary switch failed: %v\n", err)
		return
	}
	c.cfg = c.app.Config
	if err := config.Save(c.configPath, c.cfg); err != nil {
		fmt.Fprintf(c.out, "assistant> auxiliary switched but config save failed: %v\n", err)
		return
	}
	c.writeJSON(auxCfg)
}

func (c *InteractiveConsole) handleResume(ctx context.Context, rest string) {
	raw := strings.TrimSpace(rest)
	var (
		session store.Session
		err     error
	)
	if raw == "" {
		sessions, listErr := c.app.Store.ListSessions(ctx, c.user, 1)
		if listErr != nil {
			fmt.Fprintf(c.out, "assistant> resume failed: %v\n", listErr)
			return
		}
		if len(sessions) == 0 {
			fmt.Fprintln(c.out, "assistant> no sessions available to resume")
			return
		}
		session = sessions[0]
	} else {
		sessionID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || sessionID <= 0 {
			fmt.Fprintln(c.out, "assistant> usage: /resume [session-id]")
			return
		}
		session, err = c.app.Store.GetSession(ctx, sessionID)
		if err != nil {
			fmt.Fprintf(c.out, "assistant> resume failed: %v\n", err)
			return
		}
		if session.Username != c.user {
			fmt.Fprintf(c.out, "assistant> resume failed: session %d does not belong to %s\n", session.ID, c.user)
			return
		}
	}
	messages, err := c.app.Store.GetMessagesPage(ctx, session.ID, 10, 0)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> resume failed: %v\n", err)
		return
	}
	c.turnCount = 0
	c.turns = nil
	c.activeSessionID = session.ID
	c.activeSessionManaged = false
	c.lastChat = app.ChatResult{
		SessionID: session.ID,
		Model:     session.Model,
		Prompt:    session.Prompt,
		Response:  session.Response,
	}
	c.writeJSON(map[string]any{
		"resumed_session": session,
		"messages":        messages,
		"managed":         false,
		"note":            "Subsequent plain-text turns will now continue inside this session using session-scoped recent history. Undo/retry remain limited to sessions created in this console.",
	})
}

func (c *InteractiveConsole) handleRetry(ctx context.Context) {
	if len(c.turns) == 0 {
		fmt.Fprintln(c.out, "assistant> nothing to retry yet")
		return
	}
	last := c.turns[len(c.turns)-1]
	if !last.Managed {
		fmt.Fprintln(c.out, "assistant> retry only works for turns created in this console; use /new after inspecting a resumed session")
		return
	}
	if err := c.app.Store.DeleteLastTurn(ctx, last.SessionID); err != nil {
		fmt.Fprintf(c.out, "assistant> retry cleanup failed: %v\n", err)
		return
	}
	c.turns = c.turns[:len(c.turns)-1]
	if c.turnCount > 0 {
		c.turnCount--
	}
	result, err := c.app.ChatInSessionDetailed(ctx, c.user, last.SessionID, last.Prompt)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> retry failed: %v\n", err)
		return
	}
	c.recordChatTurn(result, true)
	fmt.Fprintf(c.out, "assistant> %s\n", result.Response)
}

func (c *InteractiveConsole) handleUndo(ctx context.Context) {
	if len(c.turns) == 0 {
		fmt.Fprintln(c.out, "assistant> nothing to undo yet")
		return
	}
	last := c.turns[len(c.turns)-1]
	if !last.Managed {
		fmt.Fprintln(c.out, "assistant> undo only works for turns created in this console; resumed sessions remain read-only anchors")
		return
	}
	if err := c.app.Store.DeleteLastTurn(ctx, last.SessionID); err != nil {
		fmt.Fprintf(c.out, "assistant> undo failed: %v\n", err)
		return
	}
	c.turns = c.turns[:len(c.turns)-1]
	if c.turnCount > 0 {
		c.turnCount--
	}
	if len(c.turns) > 0 {
		prev := c.turns[len(c.turns)-1]
		c.lastChat = app.ChatResult{
			SessionID: prev.SessionID,
			Model:     prev.Model,
			Prompt:    prev.Prompt,
			Response:  prev.Response,
		}
	} else {
		c.lastChat = app.ChatResult{}
	}
	if len(c.turns) == 0 || c.activeSessionID == last.SessionID {
		session, err := c.app.Store.GetSession(ctx, last.SessionID)
		if err == nil {
			c.lastChat = app.ChatResult{
				SessionID: session.ID,
				Model:     session.Model,
				Prompt:    session.Prompt,
				Response:  session.Response,
			}
			c.activeSessionID = session.ID
			c.activeSessionManaged = true
		}
	}
	if len(c.turns) == 0 && c.lastChat.SessionID == 0 {
		c.activeSessionID = 0
		c.activeSessionManaged = false
	}
	c.writeJSON(map[string]any{
		"ok":                true,
		"session_id":        last.SessionID,
		"remaining_turns":   c.turnCount,
		"active_session_id": c.activeSessionID,
	})
}

func (c *InteractiveConsole) handleContext(ctx context.Context, rest string) {
	prompt := strings.TrimSpace(rest)
	if prompt == "" {
		fmt.Fprintln(c.out, "assistant> usage: /context <prompt>")
		return
	}
	budget, err := c.app.EstimateContextBudget(ctx, c.user, prompt)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> context failed: %v\n", err)
		return
	}
	c.writeJSON(budget)
}

func (c *InteractiveConsole) handleSessions(ctx context.Context, rest string) {
	limit := parsePositiveIntDefault(strings.TrimSpace(rest), 20)
	sessions, err := c.app.Store.ListSessions(ctx, c.user, limit)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> sessions failed: %v\n", err)
		return
	}
	c.writeJSON(sessions)
}

func (c *InteractiveConsole) handleHistory(ctx context.Context, rest string) {
	messageLimit := parsePositiveIntDefault(strings.TrimSpace(rest), 10)
	payload, err := buildHistoryPayload(ctx, c.app.Store, c.user, 20, 0, messageLimit, 0)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> history failed: %v\n", err)
		return
	}
	c.writeJSON(payload)
}

func (c *InteractiveConsole) handleSearch(ctx context.Context, rest string) {
	query := strings.TrimSpace(rest)
	if query == "" {
		fmt.Fprintln(c.out, "assistant> usage: /search <query>")
		return
	}
	results, err := c.app.Store.SearchMessages(ctx, store.SearchFilters{
		Username: c.user,
		Query:    query,
		Limit:    20,
	})
	if err != nil {
		fmt.Fprintf(c.out, "assistant> search failed: %v\n", err)
		return
	}
	c.writeJSON(results)
}

func (c *InteractiveConsole) handleAudit(ctx context.Context, rest string) {
	limit := parsePositiveIntDefault(strings.TrimSpace(rest), 50)
	records, err := c.app.Store.ListAuditFiltered(ctx, store.AuditFilters{
		Username: c.user,
		Limit:    limit,
	})
	if err != nil {
		fmt.Fprintf(c.out, "assistant> audit failed: %v\n", err)
		return
	}
	c.writeJSON(records)
}

func (c *InteractiveConsole) handleExtensions(ctx context.Context) {
	states, err := c.app.Store.ListExtensionStates(ctx)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> extensions failed: %v\n", err)
		return
	}
	c.writeJSON(map[string]any{"summary": c.app.Extensions.Summary(), "states": states})
}

func (c *InteractiveConsole) handleExtensionHooks(ctx context.Context, rest string) {
	fs := flag.NewFlagSet("extension-hooks", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	kind := fs.String("kind", "", "extension kind")
	name := fs.String("name", "", "extension name")
	phase := fs.String("phase", "", "hook phase")
	limit := fs.Int("limit", 50, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	if err := parseConsoleFlagArgs(fs, rest); err != nil {
		fmt.Fprintf(c.out, "assistant> extension hooks parse failed: %v\n", err)
		return
	}
	records, err := c.app.Store.ListExtensionHookRuns(ctx, store.ExtensionHookFilters{
		Username: c.user,
		Kind:     strings.TrimSpace(*kind),
		Name:     strings.TrimSpace(*name),
		Phase:    strings.TrimSpace(*phase),
		Limit:    *limit,
		Offset:   *offset,
	})
	if err != nil {
		fmt.Fprintf(c.out, "assistant> extension hooks failed: %v\n", err)
		return
	}
	c.writeJSON(records)
}

func (c *InteractiveConsole) handleExtensionRefresh(ctx context.Context) {
	if err := c.app.Extensions.Discover(ctx); err != nil {
		fmt.Fprintf(c.out, "assistant> extension refresh failed: %v\n", err)
		return
	}
	if err := c.app.Extensions.Register(c.app.Tools); err != nil {
		fmt.Fprintf(c.out, "assistant> extension register failed: %v\n", err)
		return
	}
	c.writeJSON(c.app.Extensions.Summary())
}

func (c *InteractiveConsole) handleExtensionState(ctx context.Context, rest string) {
	fs := flag.NewFlagSet("extension-state", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	kind := fs.String("kind", "", "extension kind")
	name := fs.String("name", "", "extension name")
	enabled := fs.Bool("enabled", false, "enable or disable extension")
	if err := parseConsoleFlagArgs(fs, rest); err != nil {
		fmt.Fprintf(c.out, "assistant> extension state parse failed: %v\n", err)
		return
	}
	if strings.TrimSpace(*kind) == "" || strings.TrimSpace(*name) == "" {
		fmt.Fprintln(c.out, "assistant> usage: /extension-state --kind <kind> --name <name> --enabled <true|false>")
		return
	}
	if err := c.app.Extensions.SetEnabled(ctx, c.user, strings.TrimSpace(*kind), strings.TrimSpace(*name), *enabled); err != nil {
		fmt.Fprintf(c.out, "assistant> extension state failed: %v\n", err)
		return
	}
	if err := c.app.Extensions.Register(c.app.Tools); err != nil {
		fmt.Fprintf(c.out, "assistant> extension register failed: %v\n", err)
		return
	}
	c.writeJSON(c.app.Extensions.Summary())
}

func (c *InteractiveConsole) handleExtensionValidate(ctx context.Context, rest string) {
	fs := flag.NewFlagSet("extension-validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	kind := fs.String("kind", "", "extension kind")
	name := fs.String("name", "", "extension name")
	if err := parseConsoleFlagArgs(fs, rest); err != nil {
		fmt.Fprintf(c.out, "assistant> extension validate parse failed: %v\n", err)
		return
	}
	if strings.TrimSpace(*kind) == "" || strings.TrimSpace(*name) == "" {
		fmt.Fprintln(c.out, "assistant> usage: /extension-validate --kind <kind> --name <name>")
		return
	}
	result, err := c.app.Extensions.Validate(ctx, c.user, strings.TrimSpace(*kind), strings.TrimSpace(*name))
	if err != nil {
		fmt.Fprintf(c.out, "assistant> extension validate failed: %v\n", err)
		return
	}
	c.writeJSON(result)
}

func (c *InteractiveConsole) handleMultiAgentPlan(ctx context.Context, rest string) {
	path, objective, ok := splitAroundDoubleDash(rest)
	if !ok {
		fmt.Fprintln(c.out, "assistant> usage: /multiagent-plan <tasks-file> -- <objective>")
		return
	}
	tasks, err := loadTasksFile(path)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> load tasks failed: %v\n", err)
		return
	}
	plan, err := c.app.BuildMultiAgentPlan(ctx, c.user, objective, tasks)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent plan failed: %v\n", err)
		return
	}
	c.writeJSON(plan)
}

func (c *InteractiveConsole) handleMultiAgentRun(ctx context.Context, rest string) {
	path := strings.TrimSpace(rest)
	if path == "" {
		fmt.Fprintln(c.out, "assistant> usage: /multiagent-run <plan-file>")
		return
	}
	plan, err := loadPlanFile(path)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> load plan failed: %v\n", err)
		return
	}
	results, aggregate, err := c.app.RunMultiAgentPlan(ctx, c.user, plan)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent run failed: %v\n", err)
		return
	}
	c.writeJSON(map[string]any{"plan": plan, "results": results, "aggregate": aggregate})
}

func (c *InteractiveConsole) handleMultiAgentTraces(ctx context.Context, rest string) {
	filters, err := parseConsoleTraceFilters(rest, 50)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent traces parse failed: %v\n", err)
		return
	}
	filters.Username = c.user
	records, err := c.app.Store.ListMultiAgentTraces(ctx, filters)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent traces failed: %v\n", err)
		return
	}
	c.writeJSON(records)
}

func (c *InteractiveConsole) handleMultiAgentSummary(ctx context.Context, rest string) {
	filters, err := parseConsoleTraceFilters(rest, 0)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent summary parse failed: %v\n", err)
		return
	}
	filters.Username = c.user
	records, err := c.app.Store.SummarizeMultiAgentTraces(ctx, filters)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent summary failed: %v\n", err)
		return
	}
	c.writeJSON(records)
}

func (c *InteractiveConsole) handleMultiAgentVerifiers(ctx context.Context, rest string) {
	filters, err := parseConsoleTraceFilters(rest, 0)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent verifiers parse failed: %v\n", err)
		return
	}
	filters.Username = c.user
	records, err := c.app.Store.SummarizeMultiAgentVerifierResults(ctx, filters)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent verifiers failed: %v\n", err)
		return
	}
	c.writeJSON(records)
}

func (c *InteractiveConsole) handleMultiAgentFailures(ctx context.Context, rest string) {
	filters, err := parseConsoleTraceFilters(rest, 50)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent failures parse failed: %v\n", err)
		return
	}
	filters.Username = c.user
	records, err := c.app.Store.ListMultiAgentTraceFailures(ctx, filters)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent failures failed: %v\n", err)
		return
	}
	c.writeJSON(records)
}

func (c *InteractiveConsole) handleMultiAgentHotspots(ctx context.Context, rest string) {
	filters, err := parseConsoleTraceFilters(rest, 50)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent hotspots parse failed: %v\n", err)
		return
	}
	filters.Username = c.user
	records, err := c.app.Store.ListMultiAgentTraceHotspots(ctx, filters)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent hotspots failed: %v\n", err)
		return
	}
	c.writeJSON(records)
}

func (c *InteractiveConsole) handleMultiAgentReplay(ctx context.Context, rest string) {
	childID, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
	if err != nil || childID <= 0 {
		fmt.Fprintln(c.out, "assistant> usage: /multiagent-replay <child-session-id>")
		return
	}
	payload, err := c.app.ReplayMultiAgentChild(ctx, c.user, childID)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> replay failed: %v\n", err)
		return
	}
	c.writeJSON(payload)
}

func (c *InteractiveConsole) handleMultiAgentResume(ctx context.Context, rest string) {
	fs := flag.NewFlagSet("multiagent-resume", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	allowedTools := fs.String("allowed-tools", "", "comma-separated tool allowlist")
	historyWindow := fs.Int("history-window", 0, "history window override")
	if err := parseConsoleFlagArgs(fs, rest); err != nil {
		fmt.Fprintf(c.out, "assistant> multiagent resume parse failed: %v\n", err)
		return
	}
	args := fs.Args()
	if len(args) != 1 {
		fmt.Fprintln(c.out, "assistant> usage: /multiagent-resume <child-session-id> [--allowed-tools a,b] [--history-window n]")
		return
	}
	childID, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
	if err != nil || childID <= 0 {
		fmt.Fprintln(c.out, "assistant> usage: /multiagent-resume <child-session-id> [--allowed-tools a,b] [--history-window n]")
		return
	}
	payload, err := c.app.ResumeMultiAgentChild(ctx, c.user, childID, parseCommaList(*allowedTools), *historyWindow)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> resume failed: %v\n", err)
		return
	}
	c.writeJSON(payload)
}

func (c *InteractiveConsole) handleTrajectories(rest string) {
	parts := strings.Fields(rest)
	limit := 20
	runName := ""
	if len(parts) >= 1 {
		if parsed, err := strconv.Atoi(parts[0]); err == nil && parsed > 0 {
			limit = parsed
			if len(parts) >= 2 {
				runName = parts[1]
			}
		} else {
			runName = parts[0]
		}
	}
	manager := trajectory.NewManager(c.app.Config.DataDir)
	records, err := manager.ListFiltered(c.user, trajectory.ListFilters{
		Limit:   limit,
		RunName: runName,
	})
	if err != nil {
		fmt.Fprintf(c.out, "assistant> trajectories failed: %v\n", err)
		return
	}
	c.writeJSON(records)
}

func (c *InteractiveConsole) handleTrajectoryShow(rest string) {
	id := strings.TrimSpace(rest)
	if id == "" {
		fmt.Fprintln(c.out, "assistant> usage: /trajectory-show <trajectory-id>")
		return
	}
	manager := trajectory.NewManager(c.app.Config.DataDir)
	record, err := manager.Get(c.user, id)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> trajectory show failed: %v\n", err)
		return
	}
	c.writeJSON(record)
}

func (c *InteractiveConsole) handleTrajectorySummary(rest string) {
	manager := trajectory.NewManager(c.app.Config.DataDir)
	summary, err := manager.Summarize(c.user, trajectory.ListFilters{RunName: strings.TrimSpace(rest)})
	if err != nil {
		fmt.Fprintf(c.out, "assistant> trajectory summary failed: %v\n", err)
		return
	}
	c.writeJSON(summary)
}

func (c *InteractiveConsole) handleCronAdd(rest string) {
	left, prompt, ok := splitAroundDoubleDash(rest)
	if !ok {
		fmt.Fprintln(c.out, "assistant> usage: /cron-add <name> <schedule...> -- <prompt>")
		return
	}
	args := strings.Fields(left)
	if len(args) < 2 {
		fmt.Fprintln(c.out, "assistant> usage: /cron-add <name> <schedule...> -- <prompt>")
		return
	}
	job, err := cron.NewManager(c.app.Config.DataDir).AddJob(cron.CreateInput{
		Name:     args[0],
		Username: c.user,
		Prompt:   prompt,
		Schedule: strings.Join(args[1:], " "),
	})
	if err != nil {
		fmt.Fprintf(c.out, "assistant> cron add failed: %v\n", err)
		return
	}
	c.writeJSON(job)
}

func (c *InteractiveConsole) handleCronList() {
	jobs, err := cron.NewManager(c.app.Config.DataDir).ListJobs()
	if err != nil {
		fmt.Fprintf(c.out, "assistant> cron list failed: %v\n", err)
		return
	}
	c.writeJSON(jobs)
}

func (c *InteractiveConsole) handleCronShow(rest string) {
	id := strings.TrimSpace(rest)
	if id == "" {
		fmt.Fprintln(c.out, "assistant> usage: /cron-show <job-id>")
		return
	}
	job, err := cron.NewManager(c.app.Config.DataDir).GetJob(id)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> cron show failed: %v\n", err)
		return
	}
	c.writeJSON(job)
}

func (c *InteractiveConsole) handleCronDelete(rest string) {
	id := strings.TrimSpace(rest)
	if id == "" {
		fmt.Fprintln(c.out, "assistant> usage: /cron-delete <job-id>")
		return
	}
	if err := cron.NewManager(c.app.Config.DataDir).DeleteJob(id); err != nil {
		fmt.Fprintf(c.out, "assistant> cron delete failed: %v\n", err)
		return
	}
	c.writeJSON(map[string]any{"ok": true, "id": id})
}

func (c *InteractiveConsole) handleCronTick(ctx context.Context) {
	results, err := cron.NewManager(c.app.Config.DataDir).Tick(ctx, c.app)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> cron tick failed: %v\n", err)
		return
	}
	c.writeJSON(map[string]any{"results": results, "count": len(results)})
}

func (c *InteractiveConsole) handleToolExec(ctx context.Context, rest string) {
	name, input, err := parseToolExecInput(rest)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> tool exec parse failed: %v\n", err)
		return
	}
	if name == "" {
		fmt.Fprintln(c.out, "assistant> usage: /tool-exec <tool-name> [-- <json-object>]")
		return
	}
	input["username"] = c.user
	result, err := c.app.Tools.Execute(ctx, name, input)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> tool exec failed: %v\n", err)
		return
	}
	c.writeJSON(result)
}

func (c *InteractiveConsole) handleExecutionAudit(ctx context.Context, rest string) {
	fs := flag.NewFlagSet("execution-audit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 50, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	if err := parseConsoleFlagArgs(fs, rest); err != nil {
		fmt.Fprintf(c.out, "assistant> execution audit parse failed: %v\n", err)
		return
	}
	records, err := c.app.Store.ListAuditFiltered(ctx, store.AuditFilters{
		Username: c.user,
		Limit:    *limit,
		Offset:   *offset,
	})
	if err != nil {
		fmt.Fprintf(c.out, "assistant> execution audit failed: %v\n", err)
		return
	}
	filtered := make([]store.AuditRecord, 0, len(records))
	for _, record := range records {
		if strings.HasPrefix(record.Action, "system_exec_") {
			filtered = append(filtered, record)
		}
	}
	c.writeJSON(filtered)
}

func (c *InteractiveConsole) handleExecutionProfileAudit(ctx context.Context, rest string) {
	fs := flag.NewFlagSet("execution-profile-audit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	if err := parseConsoleFlagArgs(fs, rest); err != nil {
		fmt.Fprintf(c.out, "assistant> execution profile audit parse failed: %v\n", err)
		return
	}
	fromTime, err := parseOptionalTime("from", *from)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> execution profile audit parse failed: %v\n", err)
		return
	}
	toTime, err := parseOptionalTime("to", *to)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> execution profile audit parse failed: %v\n", err)
		return
	}
	payload, err := buildExecutionProfileAuditPayload(ctx, c.app.Store, c.user, fromTime, toTime)
	if err != nil {
		fmt.Fprintf(c.out, "assistant> execution profile audit failed: %v\n", err)
		return
	}
	c.writeJSON(payload)
}

func (c *InteractiveConsole) prompt(label string) string {
	fmt.Fprintf(c.out, "%s: ", label)
	line, _ := c.reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func (c *InteractiveConsole) writeJSON(value any) {
	var buf bytes.Buffer
	if err := writePrettyJSON(&buf, value); err != nil {
		fmt.Fprintf(c.out, "assistant> json error: %v\n", err)
		return
	}
	fmt.Fprintf(c.out, "assistant> %s", buf.String())
}

func (c *InteractiveConsole) printHelp() {
	fmt.Fprintln(c.out, "assistant> available commands:")
	fmt.Fprintln(c.out, "  /help")
	fmt.Fprintln(c.out, "  /new")
	fmt.Fprintln(c.out, "  /clear")
	fmt.Fprintln(c.out, "  /login [username] [password]")
	fmt.Fprintln(c.out, "  /whoami")
	fmt.Fprintln(c.out, "  /status")
	fmt.Fprintln(c.out, "  /usage")
	fmt.Fprintln(c.out, "  /insights [days]")
	fmt.Fprintln(c.out, "  /prompt-inspect <prompt>")
	fmt.Fprintln(c.out, "  /prompt-cache-stats")
	fmt.Fprintln(c.out, "  /prompt-cache-clear")
	fmt.Fprintln(c.out, "  /prompt-config")
	fmt.Fprintln(c.out, "  /models")
	fmt.Fprintln(c.out, "  /discover-models")
	fmt.Fprintln(c.out, "  /model <profile-or-alias>")
	fmt.Fprintln(c.out, "  /model-metadata [profile-or-alias]")
	fmt.Fprintln(c.out, "  /auxiliary-info [task]")
	fmt.Fprintln(c.out, "  /auxiliary-chat [task] -- <prompt>")
	fmt.Fprintln(c.out, "  /auxiliary-switch <profile> [default|summary|compression]")
	fmt.Fprintln(c.out, "  /resume [session-id]")
	fmt.Fprintln(c.out, "  /retry")
	fmt.Fprintln(c.out, "  /undo")
	fmt.Fprintln(c.out, "  /context <prompt>")
	fmt.Fprintln(c.out, "  /sessions [limit]")
	fmt.Fprintln(c.out, "  /history [messages-limit]")
	fmt.Fprintln(c.out, "  /search <query>")
	fmt.Fprintln(c.out, "  /audit [limit]")
	fmt.Fprintln(c.out, "  /extensions")
	fmt.Fprintln(c.out, "  /extension-hooks [--kind <kind>] [--name <name>] [--phase <phase>] [--limit n] [--offset n]")
	fmt.Fprintln(c.out, "  /extension-refresh")
	fmt.Fprintln(c.out, "  /extension-state --kind <kind> --name <name> --enabled <true|false>")
	fmt.Fprintln(c.out, "  /extension-validate --kind <kind> --name <name>")
	fmt.Fprintln(c.out, "  /tools")
	fmt.Fprintln(c.out, "  /tool-exec <tool-name> [-- <json-object>]")
	fmt.Fprintln(c.out, "  /execution-audit [--limit n] [--offset n]")
	fmt.Fprintln(c.out, "  /execution-profile-audit [--from <rfc3339>] [--to <rfc3339>]")
	fmt.Fprintln(c.out, "  /multiagent-plan <tasks-file> -- <objective>")
	fmt.Fprintln(c.out, "  /multiagent-run <plan-file>")
	fmt.Fprintln(c.out, "  /multiagent-traces [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit n] [--offset n]")
	fmt.Fprintln(c.out, "  /multiagent-summary [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>]")
	fmt.Fprintln(c.out, "  /multiagent-verifiers [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>]")
	fmt.Fprintln(c.out, "  /multiagent-failures [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit n] [--offset n]")
	fmt.Fprintln(c.out, "  /multiagent-hotspots [--parent-session-id n] [--child-session-id n] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit n] [--offset n]")
	fmt.Fprintln(c.out, "  /multiagent-replay <child-session-id>")
	fmt.Fprintln(c.out, "  /multiagent-resume <child-session-id> [--allowed-tools a,b] [--history-window n]")
	fmt.Fprintln(c.out, "  /trajectories [limit] [run-name]")
	fmt.Fprintln(c.out, "  /trajectory-show <trajectory-id>")
	fmt.Fprintln(c.out, "  /trajectory-summary [run-name]")
	fmt.Fprintln(c.out, "  /cron-add <name> <schedule...> -- <prompt>")
	fmt.Fprintln(c.out, "  /cron-list")
	fmt.Fprintln(c.out, "  /cron-show <job-id>")
	fmt.Fprintln(c.out, "  /cron-delete <job-id>")
	fmt.Fprintln(c.out, "  /cron-tick")
	fmt.Fprintln(c.out, "  /exit")
}

func splitCommandAndRest(line string) (string, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}
	command, rest, found := strings.Cut(line, " ")
	if !found {
		return command, ""
	}
	return command, strings.TrimSpace(rest)
}

func splitAroundDoubleDash(raw string) (string, string, bool) {
	left, right, found := strings.Cut(raw, " -- ")
	if !found {
		return "", "", false
	}
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return "", "", false
	}
	return left, right, true
}

func parsePositiveIntDefault(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseConsoleFlagArgs(fs *flag.FlagSet, rest string) error {
	if strings.TrimSpace(rest) == "" {
		return fs.Parse(nil)
	}
	return fs.Parse(strings.Fields(rest))
}

func parseToolExecInput(rest string) (string, map[string]any, error) {
	name := strings.TrimSpace(rest)
	input := map[string]any{}
	if left, right, ok := splitAroundDoubleDash(rest); ok {
		name = strings.TrimSpace(left)
		if err := json.Unmarshal([]byte(right), &input); err != nil {
			return "", nil, err
		}
	}
	return name, input, nil
}

func parseConsoleTraceFilters(rest string, defaultLimit int) (store.MultiAgentTraceFilters, error) {
	fs := flag.NewFlagSet("trace-filters", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	parentSessionID := fs.Int64("parent-session-id", 0, "parent session id")
	childSessionID := fs.Int64("child-session-id", 0, "child session id")
	taskID := fs.String("task-id", "", "task id")
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	limit := fs.Int("limit", defaultLimit, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	if err := parseConsoleFlagArgs(fs, rest); err != nil {
		return store.MultiAgentTraceFilters{}, err
	}
	fromTime, err := parseOptionalTime("from", *from)
	if err != nil {
		return store.MultiAgentTraceFilters{}, err
	}
	toTime, err := parseOptionalTime("to", *to)
	if err != nil {
		return store.MultiAgentTraceFilters{}, err
	}
	return store.MultiAgentTraceFilters{
		ParentSessionID: *parentSessionID,
		ChildSessionID:  *childSessionID,
		TaskID:          strings.TrimSpace(*taskID),
		FromTime:        fromTime,
		ToTime:          toTime,
		Limit:           *limit,
		Offset:          *offset,
	}, nil
}

func promptInteractiveCredentials(reader *bufio.Reader, out io.Writer) (string, string) {
	fmt.Fprint(out, "username: ")
	username, _ := reader.ReadString('\n')
	fmt.Fprint(out, "password: ")
	password, _ := reader.ReadString('\n')
	return strings.TrimSpace(username), strings.TrimSpace(password)
}

func ensureConsoleUser(ctx context.Context, application *app.App, reader *bufio.Reader, out io.Writer, username, password, token string) (string, error) {
	username = strings.TrimSpace(username)
	token = strings.TrimSpace(token)
	if username == "" && password == "" && token == "" {
		username, password = promptInteractiveCredentials(reader, out)
	}
	return authenticateChatUser(ctx, application, username, password, token)
}
