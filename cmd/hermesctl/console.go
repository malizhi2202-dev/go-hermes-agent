package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

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
	configPath string
	app        *app.App
	cfg        config.Config
	reader     *bufio.Reader
	out        io.Writer
	user       string
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
		reply, err := c.app.Chat(ctx, c.user, text)
		if err != nil {
			fmt.Fprintf(c.out, "assistant> error: %v\n", err)
			return true
		}
		fmt.Fprintf(c.out, "assistant> %s\n", reply)
		return true
	}

	commandLine := strings.TrimPrefix(text, "/")
	command, rest := splitCommandAndRest(commandLine)
	switch command {
	case "help":
		c.printHelp()
	case "login":
		c.handleLogin(ctx, rest)
	case "whoami":
		c.writeJSON(map[string]any{"username": c.user, "model": c.app.CurrentLLM().Model, "profile": c.app.CurrentModelProfile()})
	case "models":
		c.handleModels()
	case "model":
		c.handleModelSwitch(ctx, rest)
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
	case "tools":
		c.writeJSON(c.app.Tools.List())
	case "multiagent-plan":
		c.handleMultiAgentPlan(ctx, rest)
	case "multiagent-run":
		c.handleMultiAgentRun(ctx, rest)
	case "multiagent-replay":
		c.handleMultiAgentReplay(ctx, rest)
	case "trajectories":
		c.handleTrajectories(rest)
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
	default:
		fmt.Fprintf(c.out, "assistant> unknown command %q, use /help\n", command)
	}
	return true
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
	fmt.Fprintf(c.out, "assistant> logged in as %s\n", c.user)
}

func (c *InteractiveConsole) handleModels() {
	c.writeJSON(map[string]any{
		"current_profile": c.app.CurrentModelProfile(),
		"current":         c.app.CurrentLLM(),
		"profiles":        c.app.ListModelProfiles(),
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
	fmt.Fprintln(c.out, "  /login [username] [password]")
	fmt.Fprintln(c.out, "  /whoami")
	fmt.Fprintln(c.out, "  /models")
	fmt.Fprintln(c.out, "  /model <profile-or-alias>")
	fmt.Fprintln(c.out, "  /context <prompt>")
	fmt.Fprintln(c.out, "  /sessions [limit]")
	fmt.Fprintln(c.out, "  /history [messages-limit]")
	fmt.Fprintln(c.out, "  /search <query>")
	fmt.Fprintln(c.out, "  /audit [limit]")
	fmt.Fprintln(c.out, "  /extensions")
	fmt.Fprintln(c.out, "  /tools")
	fmt.Fprintln(c.out, "  /multiagent-plan <tasks-file> -- <objective>")
	fmt.Fprintln(c.out, "  /multiagent-run <plan-file>")
	fmt.Fprintln(c.out, "  /multiagent-replay <child-session-id>")
	fmt.Fprintln(c.out, "  /trajectories [limit] [run-name]")
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
