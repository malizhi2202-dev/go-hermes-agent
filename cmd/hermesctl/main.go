package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-hermes-agent/internal/app"
	"go-hermes-agent/internal/batch"
	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/cron"
	"go-hermes-agent/internal/models"
	"go-hermes-agent/internal/multiagent"
	"go-hermes-agent/internal/store"
	"go-hermes-agent/internal/trajectory"
	"go-hermes-agent/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("%s %s\n", version.AppName, version.Version)
	case "models":
		runModels(os.Args[2:])
	case "discover-models":
		runDiscoverModels(os.Args[2:])
	case "switch-model":
		runSwitchModel(os.Args[2:])
	case "init-admin":
		runInitAdmin(os.Args[2:])
	case "login":
		runLogin(os.Args[2:])
	case "chat":
		runChat(os.Args[2:])
	case "context":
		runContext(os.Args[2:])
	case "prompt-inspect":
		runPromptInspect(os.Args[2:])
	case "prompt-cache-stats":
		runPromptCacheStats(os.Args[2:])
	case "prompt-cache-clear":
		runPromptCacheClear(os.Args[2:])
	case "prompt-config":
		runPromptConfig(os.Args[2:])
	case "auxiliary-info":
		runAuxiliaryInfo(os.Args[2:])
	case "auxiliary-chat":
		runAuxiliaryChat(os.Args[2:])
	case "auxiliary-switch":
		runAuxiliarySwitch(os.Args[2:])
	case "model-metadata":
		runModelMetadata(os.Args[2:])
	case "batch-run":
		runBatchRun(os.Args[2:])
	case "cron-add":
		runCronAdd(os.Args[2:])
	case "cron-list":
		runCronList(os.Args[2:])
	case "cron-show":
		runCronShow(os.Args[2:])
	case "cron-delete":
		runCronDelete(os.Args[2:])
	case "cron-tick":
		runCronTick(os.Args[2:])
	case "trajectories":
		runTrajectories(os.Args[2:])
	case "trajectory-summary":
		runTrajectorySummary(os.Args[2:])
	case "trajectory-show":
		runTrajectoryShow(os.Args[2:])
	case "sessions":
		runSessions(os.Args[2:])
	case "history":
		runHistory(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "audit":
		runAudit(os.Args[2:])
	case "execution-audit":
		runExecutionAudit(os.Args[2:])
	case "execution-profile-audit":
		runExecutionProfileAudit(os.Args[2:])
	case "extensions":
		runExtensions(os.Args[2:])
	case "extension-hooks":
		runExtensionHooks(os.Args[2:])
	case "extension-refresh":
		runRefreshExtensions(os.Args[2:])
	case "extension-state":
		runExtensionState(os.Args[2:])
	case "extension-validate":
		runExtensionValidate(os.Args[2:])
	case "tools":
		runTools(os.Args[2:])
	case "tool-exec":
		runExecuteTool(os.Args[2:])
	case "multiagent-plan":
		runMultiAgentPlan(os.Args[2:])
	case "multiagent-run":
		runMultiAgentRun(os.Args[2:])
	case "multiagent-traces":
		runMultiAgentTraces(os.Args[2:])
	case "multiagent-summary":
		runMultiAgentTraceSummary(os.Args[2:])
	case "multiagent-verifiers":
		runMultiAgentVerifierSummary(os.Args[2:])
	case "multiagent-failures":
		runMultiAgentTraceFailures(os.Args[2:])
	case "multiagent-hotspots":
		runMultiAgentTraceHotspots(os.Args[2:])
	case "multiagent-replay":
		runMultiAgentReplay(os.Args[2:])
	case "multiagent-resume":
		runMultiAgentResume(os.Args[2:])
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("usage:")
	fmt.Println("  hermesctl version")
	fmt.Println("  hermesctl models --config <file>")
	fmt.Println("  hermesctl discover-models --config <file>")
	fmt.Println("  hermesctl switch-model --config <file> --profile <name>")
	fmt.Println("  hermesctl switch-model --config <file> --model <id> --base-url <url> [--provider <name>] [--api-key-env <ENV>] [--local]")
	fmt.Println("  hermesctl init-admin --config <file> --username <user> --password <pass>")
	fmt.Println("  hermesctl login --config <file> --username <user> --password <pass>")
	fmt.Println("  hermesctl chat --config <file> (--username <user> --password <pass> | --token <jwt>) [--prompt <text>]")
	fmt.Println("  hermesctl context --config <file> (--username <user> --password <pass> | --token <jwt>) --prompt <text>")
	fmt.Println("  hermesctl sessions --config <file> (--username <user> --password <pass> | --token <jwt>) [--limit <n>] [--offset <n>]")
	fmt.Println("  hermesctl prompt-inspect --config <file> (--username <user> --password <pass> | --token <jwt>) --prompt <text>")
	fmt.Println("  hermesctl prompt-cache-stats --config <file> (--username <user> --password <pass> | --token <jwt>)")
	fmt.Println("  hermesctl prompt-cache-clear --config <file> (--username <user> --password <pass> | --token <jwt>)")
	fmt.Println("  hermesctl prompt-config --config <file>")
	fmt.Println("  hermesctl auxiliary-info --config <file> [--task <summary|compression|general>]")
	fmt.Println("  hermesctl auxiliary-chat --config <file> [--task <summary|compression|general>] --prompt <text>")
	fmt.Println("  hermesctl auxiliary-switch --config <file> --profile <name> [--task <default|summary|compression>]")
	fmt.Println("  hermesctl model-metadata --config <file> [--profile <name>]")
	fmt.Println("  hermesctl batch-run --config <file> (--username <user> --password <pass> | --token <jwt>) --dataset-file <path> [--run-name <name>]")
	fmt.Println("  hermesctl cron-add --config <file> (--username <user> --password <pass> | --token <jwt>) --name <name> --prompt <text> --schedule <expr> [--run-as <user>]")
	fmt.Println("  hermesctl cron-list --config <file> (--username <user> --password <pass> | --token <jwt>)")
	fmt.Println("  hermesctl cron-show --config <file> (--username <user> --password <pass> | --token <jwt>) --id <job-id>")
	fmt.Println("  hermesctl cron-delete --config <file> (--username <user> --password <pass> | --token <jwt>) --id <job-id>")
	fmt.Println("  hermesctl cron-tick --config <file> (--username <user> --password <pass> | --token <jwt>)")
	fmt.Println("  hermesctl trajectories --config <file> (--username <user> --password <pass> | --token <jwt>) [--limit <n>]")
	fmt.Println("  hermesctl trajectory-summary --config <file> (--username <user> --password <pass> | --token <jwt>) [--run-name <name>] [--model <name>] [--source <name>] [--completed <true|false>]")
	fmt.Println("  hermesctl trajectory-show --config <file> (--username <user> --password <pass> | --token <jwt>) --id <trajectory-id>")
	fmt.Println("  hermesctl history --config <file> (--username <user> --password <pass> | --token <jwt>) [--limit <n>] [--offset <n>] [--messages-limit <n>] [--messages-offset <n>]")
	fmt.Println("  hermesctl search --config <file> (--username <user> --password <pass> | --token <jwt>) --q <query> [--role <role>] [--session-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit <n>]")
	fmt.Println("  hermesctl audit --config <file> (--username <user> --password <pass> | --token <jwt>) [--action <name>] [--from <rfc3339>] [--to <rfc3339>] [--limit <n>] [--offset <n>]")
	fmt.Println("  hermesctl execution-audit --config <file> (--username <user> --password <pass> | --token <jwt>) [--limit <n>] [--offset <n>]")
	fmt.Println("  hermesctl execution-profile-audit --config <file> (--username <user> --password <pass> | --token <jwt>) [--from <rfc3339>] [--to <rfc3339>]")
	fmt.Println("  hermesctl extensions --config <file> (--username <user> --password <pass> | --token <jwt>)")
	fmt.Println("  hermesctl extension-hooks --config <file> (--username <user> --password <pass> | --token <jwt>) [--kind <kind>] [--name <name>] [--phase <phase>] [--limit <n>] [--offset <n>]")
	fmt.Println("  hermesctl extension-refresh --config <file> (--username <user> --password <pass> | --token <jwt>)")
	fmt.Println("  hermesctl extension-state --config <file> (--username <user> --password <pass> | --token <jwt>) --kind <kind> --name <name> --enabled <true|false>")
	fmt.Println("  hermesctl extension-validate --config <file> (--username <user> --password <pass> | --token <jwt>) --kind <kind> --name <name>")
	fmt.Println("  hermesctl tools --config <file> (--username <user> --password <pass> | --token <jwt>)")
	fmt.Println("  hermesctl tool-exec --config <file> (--username <user> --password <pass> | --token <jwt>) --name <tool> [--input-json <json> | --input-file <path>]")
	fmt.Println("  hermesctl multiagent-plan --config <file> (--username <user> --password <pass> | --token <jwt>) --objective <text> --tasks-file <path>")
	fmt.Println("  hermesctl multiagent-run --config <file> (--username <user> --password <pass> | --token <jwt>) --plan-file <path>")
	fmt.Println("  hermesctl multiagent-traces --config <file> (--username <user> --password <pass> | --token <jwt>) [--parent-session-id <id>] [--child-session-id <id>] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit <n>] [--offset <n>]")
	fmt.Println("  hermesctl multiagent-summary --config <file> (--username <user> --password <pass> | --token <jwt>) [--parent-session-id <id>] [--child-session-id <id>] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>]")
	fmt.Println("  hermesctl multiagent-verifiers --config <file> (--username <user> --password <pass> | --token <jwt>) [--parent-session-id <id>] [--child-session-id <id>] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>]")
	fmt.Println("  hermesctl multiagent-failures --config <file> (--username <user> --password <pass> | --token <jwt>) [--parent-session-id <id>] [--child-session-id <id>] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit <n>] [--offset <n>]")
	fmt.Println("  hermesctl multiagent-hotspots --config <file> (--username <user> --password <pass> | --token <jwt>) [--parent-session-id <id>] [--child-session-id <id>] [--task-id <id>] [--from <rfc3339>] [--to <rfc3339>] [--limit <n>] [--offset <n>]")
	fmt.Println("  hermesctl multiagent-replay --config <file> (--username <user> --password <pass> | --token <jwt>) --child-session-id <id>")
	fmt.Println("  hermesctl multiagent-resume --config <file> (--username <user> --password <pass> | --token <jwt>) --child-session-id <id> [--allowed-tools <a,b>] [--history-window <n>]")
}

func runModels(args []string) {
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	current := cfg.ResolvedLLM()
	fmt.Printf("current profile: %s\n", cfg.CurrentModelProfile)
	fmt.Printf("current model:   %s\n", current.Model)
	fmt.Printf("current base:    %s\n", current.BaseURL)
	fmt.Println("")
	fmt.Println("available profiles:")
	profiles := cfg.ListModelProfiles()
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		profile := profiles[name]
		localText := ""
		if profile.Local {
			localText = " [local]"
		}
		label := profile.DisplayName
		if label == "" {
			label = profile.Model
		}
		fmt.Printf("  %s -> %s (%s)%s\n", name, label, profile.BaseURL, localText)
	}
}

func runDiscoverModels(args []string) {
	fs := flag.NewFlagSet("discover-models", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	discovered, err := models.DiscoverLocalModels(context.Background(), cfg.ListModelProfiles())
	if err != nil {
		log.Fatalf("discover models: %v", err)
	}
	if len(discovered) == 0 {
		fmt.Println("no local models discovered")
		return
	}
	fmt.Println("discovered local models:")
	for _, model := range discovered {
		fmt.Printf("  %s -> %s (%s)\n", model.ProfileName, model.Model, model.BaseURL)
	}
}

func runSwitchModel(args []string) {
	fs := flag.NewFlagSet("switch-model", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	profile := fs.String("profile", "", "model profile name")
	modelID := fs.String("model", "", "direct model id")
	baseURL := fs.String("base-url", "", "direct model base url")
	provider := fs.String("provider", "openai-compatible", "direct model provider")
	apiKey := fs.String("api-key", "", "direct model API key")
	local := fs.Bool("local", false, "mark direct model as local")
	_ = fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *modelID != "" {
		if *baseURL == "" {
			log.Fatal("base-url is required when using --model")
		}
		profileName := strings.TrimSpace(*profile)
		if profileName == "" {
			profileName = "custom-" + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(*modelID)), ":", "-")
		}
		llmCfg := cfg.ResolvedLLM()
		llmCfg.Model = strings.TrimSpace(*modelID)
		llmCfg.BaseURL = strings.TrimSpace(*baseURL)
		llmCfg.Provider = strings.TrimSpace(*provider)
		llmCfg.APIKey = strings.TrimSpace(*apiKey)
		llmCfg.Local = *local
		llmCfg.DisplayName = *modelID
		if err := cfg.UpsertModelProfile(profileName, llmCfg); err != nil {
			log.Fatalf("switch model: %v", err)
		}
		if err := config.Save(*configPath, cfg); err != nil {
			log.Fatalf("save config: %v", err)
		}
		current := cfg.ResolvedLLM()
		fmt.Printf("switched model profile to %q\n", profileName)
		fmt.Printf("model: %s\n", current.Model)
		fmt.Printf("base:  %s\n", current.BaseURL)
		return
	}
	if *profile == "" {
		log.Fatal("profile or model is required")
	}
	profileName := *profile
	if resolved, ok := models.DefaultCatalog().ResolveProfile(cfg.ListModelProfiles(), *profile); ok {
		profileName = resolved
	}
	if err := cfg.UseModelProfile(profileName); err != nil {
		log.Fatalf("switch model: %v", err)
	}
	if err := config.Save(*configPath, cfg); err != nil {
		log.Fatalf("save config: %v", err)
	}
	current := cfg.ResolvedLLM()
	fmt.Printf("switched model profile to %q\n", profileName)
	fmt.Printf("model: %s\n", current.Model)
	fmt.Printf("base:  %s\n", current.BaseURL)
}

func runInitAdmin(args []string) {
	fs := flag.NewFlagSet("init-admin", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "admin username")
	password := fs.String("password", "", "admin password")
	_ = fs.Parse(args)

	cfg, application := mustApp(*configPath)
	defer application.Close()
	if *username == "" || *password == "" {
		log.Fatal("username and password are required")
	}
	if err := application.Auth.InitAdmin(context.Background(), *username, *password); err != nil {
		log.Fatalf("init admin: %v", err)
	}
	fmt.Printf("admin user %q created in %s\n", *username, cfg.DBPath())
}

func runLogin(args []string) {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	if *username == "" || *password == "" {
		log.Fatal("username and password are required")
	}
	token, err := application.Auth.Login(context.Background(), *username, *password)
	if err != nil {
		log.Fatalf("login failed: %v", err)
	}
	fmt.Println(token)
}

func runChat(args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	prompt := fs.String("prompt", "", "single prompt to send")
	_ = fs.Parse(args)

	cfg, application := mustApp(*configPath)
	defer application.Close()

	if text := strings.TrimSpace(*prompt); text != "" {
		chatUser, err := authenticateChatUser(context.Background(), application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
		if err != nil {
			log.Fatalf("chat auth: %v", err)
		}
		reply, err := application.Chat(context.Background(), chatUser, text)
		if err != nil {
			log.Fatalf("chat failed: %v", err)
		}
		fmt.Println(reply)
		return
	}

	if piped := strings.TrimSpace(readAllFromStdin()); piped != "" {
		chatUser, err := authenticateChatUser(context.Background(), application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
		if err != nil {
			log.Fatalf("chat auth: %v", err)
		}
		reply, err := application.Chat(context.Background(), chatUser, piped)
		if err != nil {
			log.Fatalf("chat failed: %v", err)
		}
		fmt.Println(reply)
		return
	}

	reader := bufio.NewReader(os.Stdin)
	chatUser, err := ensureConsoleUser(context.Background(), application, reader, os.Stdout, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if err != nil {
		log.Fatalf("chat auth: %v", err)
	}
	console := newInteractiveConsole(*configPath, cfg, application, reader, os.Stdout, chatUser)
	console.Start(context.Background())
}

func runContext(args []string) {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	prompt := fs.String("prompt", "", "prompt to estimate")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser, err := authenticateChatUser(context.Background(), application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if err != nil {
		log.Fatalf("context auth: %v", err)
	}
	if strings.TrimSpace(*prompt) == "" {
		log.Fatal("prompt is required")
	}
	budget, err := application.EstimateContextBudget(context.Background(), chatUser, strings.TrimSpace(*prompt))
	if err != nil {
		log.Fatalf("estimate context: %v", err)
	}
	mustWriteJSON(os.Stdout, budget)
}

func runPromptInspect(args []string) {
	fs := flag.NewFlagSet("prompt-inspect", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	prompt := fs.String("prompt", "", "prompt to inspect")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*prompt) == "" {
		log.Fatal("prompt is required")
	}
	plan, err := application.InspectPrompt(context.Background(), chatUser, strings.TrimSpace(*prompt))
	if err != nil {
		log.Fatalf("inspect prompt: %v", err)
	}
	mustWriteJSON(os.Stdout, plan)
}

func runPromptCacheStats(args []string) {
	fs := flag.NewFlagSet("prompt-cache-stats", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	_ = mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	mustWriteJSON(os.Stdout, application.PromptCacheStats())
}

func runPromptCacheClear(args []string) {
	fs := flag.NewFlagSet("prompt-cache-clear", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	application.ClearPromptCache()
	_ = application.Store.WriteAudit(context.Background(), chatUser, "prompt_cache_cleared", "cli")
	mustWriteJSON(os.Stdout, map[string]any{"ok": true, "stats": application.PromptCacheStats()})
}

func runPromptConfig(args []string) {
	fs := flag.NewFlagSet("prompt-config", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	_ = fs.Parse(args)
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	mustWriteJSON(os.Stdout, map[string]any{"context": cfg.Context, "prompting": cfg.Prompting, "auxiliary": cfg.Auxiliary})
}

func runAuxiliaryInfo(args []string) {
	fs := flag.NewFlagSet("auxiliary-info", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	task := fs.String("task", "general", "auxiliary task")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	info, err := application.AuxiliaryInfo(strings.TrimSpace(*task))
	if err != nil {
		log.Fatalf("auxiliary info: %v", err)
	}
	mustWriteJSON(os.Stdout, info)
}

func runAuxiliaryChat(args []string) {
	fs := flag.NewFlagSet("auxiliary-chat", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	task := fs.String("task", "general", "auxiliary task")
	prompt := fs.String("prompt", "", "auxiliary prompt")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	if strings.TrimSpace(*prompt) == "" {
		log.Fatal("prompt is required")
	}
	response, info, err := application.AuxiliaryChat(context.Background(), strings.TrimSpace(*task), strings.TrimSpace(*prompt))
	if err != nil {
		log.Fatalf("auxiliary chat: %v", err)
	}
	mustWriteJSON(os.Stdout, map[string]any{"resolution": info, "response": response})
}

func runAuxiliarySwitch(args []string) {
	fs := flag.NewFlagSet("auxiliary-switch", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	profile := fs.String("profile", "", "model profile name")
	task := fs.String("task", "default", "default, summary, or compression")
	_ = fs.Parse(args)
	if strings.TrimSpace(*profile) == "" {
		log.Fatal("profile is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if _, ok := cfg.ModelProfiles[strings.TrimSpace(*profile)]; !ok {
		log.Fatalf("profile %q is not defined", strings.TrimSpace(*profile))
	}
	switch strings.ToLower(strings.TrimSpace(*task)) {
	case "default":
		cfg.Auxiliary.Profile = strings.TrimSpace(*profile)
	case "summary":
		cfg.Auxiliary.SummaryProfile = strings.TrimSpace(*profile)
	case "compression":
		cfg.Auxiliary.CompressionProfile = strings.TrimSpace(*profile)
	default:
		log.Fatal("task must be one of: default, summary, compression")
	}
	if err := config.Save(*configPath, cfg); err != nil {
		log.Fatalf("save config: %v", err)
	}
	mustWriteJSON(os.Stdout, cfg.Auxiliary)
}

func runModelMetadata(args []string) {
	fs := flag.NewFlagSet("model-metadata", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	profile := fs.String("profile", "", "model profile name")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	if strings.TrimSpace(*profile) == "" {
		mustWriteJSON(os.Stdout, map[string]any{"current": application.CurrentModelMetadata(), "profiles": application.ListModelMetadata()})
		return
	}
	profiles := application.ListModelProfiles()
	name := strings.TrimSpace(*profile)
	if resolved, ok := models.DefaultCatalog().ResolveProfile(profiles, name); ok {
		name = resolved
	}
	profileCfg, ok := profiles[name]
	if !ok {
		log.Fatalf("profile %q is not defined", name)
	}
	mustWriteJSON(os.Stdout, models.ResolveMetadata(profileCfg))
}

func runBatchRun(args []string) {
	fs := flag.NewFlagSet("batch-run", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	datasetFile := fs.String("dataset-file", "", "jsonl dataset file")
	runName := fs.String("run-name", "batch-run", "run name")
	resume := fs.Bool("resume", false, "resume from saved checkpoint")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*datasetFile) == "" {
		log.Fatal("dataset-file is required")
	}
	items, err := batch.LoadJSONL(strings.TrimSpace(*datasetFile))
	if err != nil {
		log.Fatalf("load batch dataset: %v", err)
	}
	manager := trajectory.NewManager(application.Config.DataDir)
	runner := batch.NewRunner(application, manager)
	results, summary, err := runner.RunWithOptions(context.Background(), chatUser, strings.TrimSpace(*runName), items, batch.RunOptions{
		DatasetFile: strings.TrimSpace(*datasetFile),
		Resume:      *resume,
	})
	if err != nil {
		log.Fatalf("run batch: %v", err)
	}
	mustWriteJSON(os.Stdout, map[string]any{
		"summary": summary,
		"results": results,
	})
}

func runCronAdd(args []string) {
	fs := flag.NewFlagSet("cron-add", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	name := fs.String("name", "", "job name")
	prompt := fs.String("prompt", "", "job prompt")
	schedule := fs.String("schedule", "", "schedule expression")
	runAs := fs.String("run-as", "", "run as username")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	jobUser := strings.TrimSpace(*runAs)
	if jobUser == "" {
		jobUser = chatUser
	}
	manager := cron.NewManager(application.Config.DataDir)
	job, err := manager.AddJob(cron.CreateInput{
		Name:     strings.TrimSpace(*name),
		Username: jobUser,
		Prompt:   strings.TrimSpace(*prompt),
		Schedule: strings.TrimSpace(*schedule),
	})
	if err != nil {
		log.Fatalf("add cron job: %v", err)
	}
	mustWriteJSON(os.Stdout, job)
}

func runCronList(args []string) {
	fs := flag.NewFlagSet("cron-list", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	_ = mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	manager := cron.NewManager(application.Config.DataDir)
	jobs, err := manager.ListJobs()
	if err != nil {
		log.Fatalf("list cron jobs: %v", err)
	}
	mustWriteJSON(os.Stdout, jobs)
}

func runCronShow(args []string) {
	fs := flag.NewFlagSet("cron-show", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	id := fs.String("id", "", "job id")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	_ = mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	manager := cron.NewManager(application.Config.DataDir)
	job, err := manager.GetJob(strings.TrimSpace(*id))
	if err != nil {
		log.Fatalf("get cron job: %v", err)
	}
	mustWriteJSON(os.Stdout, job)
}

func runCronDelete(args []string) {
	fs := flag.NewFlagSet("cron-delete", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	id := fs.String("id", "", "job id")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	_ = mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	manager := cron.NewManager(application.Config.DataDir)
	if err := manager.DeleteJob(strings.TrimSpace(*id)); err != nil {
		log.Fatalf("delete cron job: %v", err)
	}
	mustWriteJSON(os.Stdout, map[string]any{"ok": true, "id": strings.TrimSpace(*id)})
}

func runCronTick(args []string) {
	fs := flag.NewFlagSet("cron-tick", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	_ = mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	manager := cron.NewManager(application.Config.DataDir)
	results, err := manager.Tick(context.Background(), application)
	if err != nil {
		log.Fatalf("tick cron jobs: %v", err)
	}
	mustWriteJSON(os.Stdout, map[string]any{"results": results, "count": len(results)})
}

func runTrajectories(args []string) {
	fs := flag.NewFlagSet("trajectories", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	limit := fs.Int("limit", 20, "trajectory limit")
	runName := fs.String("run-name", "", "run name filter")
	model := fs.String("model", "", "model filter")
	source := fs.String("source", "", "source filter")
	completed := fs.String("completed", "", "completed filter")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	manager := trajectory.NewManager(application.Config.DataDir)
	records, err := manager.ListFiltered(chatUser, trajectory.ListFilters{
		Limit:     *limit,
		RunName:   strings.TrimSpace(*runName),
		Model:     strings.TrimSpace(*model),
		Source:    strings.TrimSpace(*source),
		Completed: parseOptionalBoolPointer(*completed),
	})
	if err != nil {
		log.Fatalf("list trajectories: %v", err)
	}
	mustWriteJSON(os.Stdout, records)
}

func runTrajectorySummary(args []string) {
	fs := flag.NewFlagSet("trajectory-summary", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	runName := fs.String("run-name", "", "run name filter")
	model := fs.String("model", "", "model filter")
	source := fs.String("source", "", "source filter")
	completed := fs.String("completed", "", "completed filter")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	manager := trajectory.NewManager(application.Config.DataDir)
	summary, err := manager.Summarize(chatUser, trajectory.ListFilters{
		RunName:   strings.TrimSpace(*runName),
		Model:     strings.TrimSpace(*model),
		Source:    strings.TrimSpace(*source),
		Completed: parseOptionalBoolPointer(*completed),
	})
	if err != nil {
		log.Fatalf("summarize trajectories: %v", err)
	}
	mustWriteJSON(os.Stdout, summary)
}

func runTrajectoryShow(args []string) {
	fs := flag.NewFlagSet("trajectory-show", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	id := fs.String("id", "", "trajectory id")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*id) == "" {
		log.Fatal("id is required")
	}
	manager := trajectory.NewManager(application.Config.DataDir)
	record, err := manager.Get(chatUser, strings.TrimSpace(*id))
	if err != nil {
		log.Fatalf("get trajectory: %v", err)
	}
	mustWriteJSON(os.Stdout, record)
}

func runSessions(args []string) {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	limit := fs.Int("limit", 20, "session limit")
	offset := fs.Int("offset", 0, "session offset")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	sessions, err := application.Store.ListSessionsPage(context.Background(), chatUser, *limit, *offset)
	if err != nil {
		log.Fatalf("list sessions: %v", err)
	}
	mustWriteJSON(os.Stdout, sessions)
}

func runHistory(args []string) {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	limit := fs.Int("limit", 20, "session limit")
	offset := fs.Int("offset", 0, "session offset")
	messagesLimit := fs.Int("messages-limit", 20, "message limit per session")
	messagesOffset := fs.Int("messages-offset", 0, "message offset per session")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	payload, err := buildHistoryPayload(context.Background(), application.Store, chatUser, *limit, *offset, *messagesLimit, *messagesOffset)
	if err != nil {
		log.Fatalf("load history: %v", err)
	}
	mustWriteJSON(os.Stdout, payload)
}

func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	query := fs.String("q", "", "search query")
	role := fs.String("role", "", "role filter")
	sessionID := fs.Int64("session-id", 0, "session id filter")
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	limit := fs.Int("limit", 20, "result limit")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*query) == "" {
		log.Fatal("q is required")
	}
	fromTime, err := parseOptionalTime("from", *from)
	if err != nil {
		log.Fatal(err)
	}
	toTime, err := parseOptionalTime("to", *to)
	if err != nil {
		log.Fatal(err)
	}
	results, err := application.Store.SearchMessages(context.Background(), store.SearchFilters{
		Username:  chatUser,
		Query:     strings.TrimSpace(*query),
		Role:      strings.TrimSpace(*role),
		SessionID: *sessionID,
		FromTime:  fromTime,
		ToTime:    toTime,
		Limit:     *limit,
	})
	if err != nil {
		log.Fatalf("search history: %v", err)
	}
	mustWriteJSON(os.Stdout, results)
}

func runAudit(args []string) {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	action := fs.String("action", "", "action filter")
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	limit := fs.Int("limit", 50, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	fromTime, err := parseOptionalTime("from", *from)
	if err != nil {
		log.Fatal(err)
	}
	toTime, err := parseOptionalTime("to", *to)
	if err != nil {
		log.Fatal(err)
	}
	records, err := application.Store.ListAuditFiltered(context.Background(), store.AuditFilters{
		Username: chatUser,
		Action:   strings.TrimSpace(*action),
		FromTime: fromTime,
		ToTime:   toTime,
		Limit:    *limit,
		Offset:   *offset,
	})
	if err != nil {
		log.Fatalf("list audit: %v", err)
	}
	mustWriteJSON(os.Stdout, records)
}

func runExecutionAudit(args []string) {
	fs := flag.NewFlagSet("execution-audit", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	limit := fs.Int("limit", 50, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	records, err := application.Store.ListAuditFiltered(context.Background(), store.AuditFilters{Username: chatUser, Limit: *limit, Offset: *offset})
	if err != nil {
		log.Fatalf("list execution audit: %v", err)
	}
	filtered := make([]store.AuditRecord, 0, len(records))
	for _, record := range records {
		if strings.HasPrefix(record.Action, "system_exec_") {
			filtered = append(filtered, record)
		}
	}
	mustWriteJSON(os.Stdout, filtered)
}

func runExecutionProfileAudit(args []string) {
	fs := flag.NewFlagSet("execution-profile-audit", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	fromTime, err := parseOptionalTime("from", *from)
	if err != nil {
		log.Fatal(err)
	}
	toTime, err := parseOptionalTime("to", *to)
	if err != nil {
		log.Fatal(err)
	}
	payload, err := buildExecutionProfileAuditPayload(context.Background(), application.Store, chatUser, fromTime, toTime)
	if err != nil {
		log.Fatalf("list execution profile audit: %v", err)
	}
	mustWriteJSON(os.Stdout, payload)
}

func runExtensions(args []string) {
	fs := flag.NewFlagSet("extensions", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	_ = mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	states, err := application.Store.ListExtensionStates(context.Background())
	if err != nil {
		log.Fatalf("list extension states: %v", err)
	}
	mustWriteJSON(os.Stdout, map[string]any{"summary": application.Extensions.Summary(), "states": states})
}

func runExtensionHooks(args []string) {
	fs := flag.NewFlagSet("extension-hooks", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	kind := fs.String("kind", "", "extension kind")
	name := fs.String("name", "", "extension name")
	phase := fs.String("phase", "", "hook phase")
	limit := fs.Int("limit", 50, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	records, err := application.Store.ListExtensionHookRuns(context.Background(), store.ExtensionHookFilters{
		Username: chatUser,
		Kind:     strings.TrimSpace(*kind),
		Name:     strings.TrimSpace(*name),
		Phase:    strings.TrimSpace(*phase),
		Limit:    *limit,
		Offset:   *offset,
	})
	if err != nil {
		log.Fatalf("list extension hooks: %v", err)
	}
	mustWriteJSON(os.Stdout, records)
}

func runRefreshExtensions(args []string) {
	fs := flag.NewFlagSet("extension-refresh", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	_ = mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if err := application.Extensions.Discover(context.Background()); err != nil {
		log.Fatalf("refresh extensions: %v", err)
	}
	if err := application.Extensions.Register(application.Tools); err != nil {
		log.Fatalf("register refreshed extensions: %v", err)
	}
	mustWriteJSON(os.Stdout, application.Extensions.Summary())
}

func runExtensionState(args []string) {
	fs := flag.NewFlagSet("extension-state", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	kind := fs.String("kind", "", "extension kind")
	name := fs.String("name", "", "extension name")
	enabled := fs.Bool("enabled", false, "enable or disable extension")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*kind) == "" || strings.TrimSpace(*name) == "" {
		log.Fatal("kind and name are required")
	}
	if err := application.Extensions.SetEnabled(context.Background(), chatUser, strings.TrimSpace(*kind), strings.TrimSpace(*name), *enabled); err != nil {
		log.Fatalf("set extension state: %v", err)
	}
	if err := application.Extensions.Register(application.Tools); err != nil {
		log.Fatalf("register extensions: %v", err)
	}
	mustWriteJSON(os.Stdout, application.Extensions.Summary())
}

func runExtensionValidate(args []string) {
	fs := flag.NewFlagSet("extension-validate", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	kind := fs.String("kind", "", "extension kind")
	name := fs.String("name", "", "extension name")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*kind) == "" || strings.TrimSpace(*name) == "" {
		log.Fatal("kind and name are required")
	}
	result, err := application.Extensions.Validate(context.Background(), chatUser, strings.TrimSpace(*kind), strings.TrimSpace(*name))
	if err != nil {
		log.Fatalf("validate extension: %v", err)
	}
	mustWriteJSON(os.Stdout, result)
}

func runTools(args []string) {
	fs := flag.NewFlagSet("tools", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	_ = mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	mustWriteJSON(os.Stdout, application.Tools.List())
}

func runExecuteTool(args []string) {
	fs := flag.NewFlagSet("tool-exec", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	name := fs.String("name", "", "tool name")
	inputJSON := fs.String("input-json", "", "inline json object")
	inputFile := fs.String("input-file", "", "json file containing tool input")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*name) == "" {
		log.Fatal("name is required")
	}
	input := map[string]any{"username": chatUser}
	if strings.TrimSpace(*inputJSON) != "" {
		if err := json.Unmarshal([]byte(*inputJSON), &input); err != nil {
			log.Fatalf("decode input-json: %v", err)
		}
		input["username"] = chatUser
	} else if strings.TrimSpace(*inputFile) != "" {
		if err := loadJSONFile(*inputFile, &input); err != nil {
			log.Fatalf("load input file: %v", err)
		}
		input["username"] = chatUser
	}
	result, err := application.Tools.Execute(context.Background(), strings.TrimSpace(*name), input)
	if err != nil {
		log.Fatalf("execute tool: %v", err)
	}
	mustWriteJSON(os.Stdout, result)
}

func runMultiAgentPlan(args []string) {
	fs := flag.NewFlagSet("multiagent-plan", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	objective := fs.String("objective", "", "plan objective")
	tasksFile := fs.String("tasks-file", "", "json file containing task list")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*objective) == "" || strings.TrimSpace(*tasksFile) == "" {
		log.Fatal("objective and tasks-file are required")
	}
	tasks, err := loadTasksFile(*tasksFile)
	if err != nil {
		log.Fatalf("load tasks: %v", err)
	}
	plan, err := application.BuildMultiAgentPlan(context.Background(), chatUser, strings.TrimSpace(*objective), tasks)
	if err != nil {
		log.Fatalf("build multiagent plan: %v", err)
	}
	mustWriteJSON(os.Stdout, plan)
}

func runMultiAgentRun(args []string) {
	fs := flag.NewFlagSet("multiagent-run", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	planFile := fs.String("plan-file", "", "json file containing a multiagent plan")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if strings.TrimSpace(*planFile) == "" {
		log.Fatal("plan-file is required")
	}
	plan, err := loadPlanFile(*planFile)
	if err != nil {
		log.Fatalf("load plan: %v", err)
	}
	results, aggregate, err := application.RunMultiAgentPlan(context.Background(), chatUser, plan)
	if err != nil {
		log.Fatalf("run multiagent plan: %v", err)
	}
	mustWriteJSON(os.Stdout, map[string]any{"plan": plan, "results": results, "aggregate": aggregate})
}

func runMultiAgentTraces(args []string) {
	fs := flag.NewFlagSet("multiagent-traces", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	parentSessionID := fs.Int64("parent-session-id", 0, "parent session id")
	childSessionID := fs.Int64("child-session-id", 0, "child session id")
	taskID := fs.String("task-id", "", "task id")
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	limit := fs.Int("limit", 50, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	_ = fs.Parse(args)

	filters, application := mustTraceFiltersAndApp(*configPath, strings.TrimSpace(*username), *password, strings.TrimSpace(*token), *parentSessionID, *childSessionID, strings.TrimSpace(*taskID), *from, *to, *limit, *offset)
	defer application.Close()
	records, err := application.Store.ListMultiAgentTraces(context.Background(), filters)
	if err != nil {
		log.Fatalf("list multiagent traces: %v", err)
	}
	mustWriteJSON(os.Stdout, records)
}

func runMultiAgentTraceSummary(args []string) {
	filters, application := parseTraceCommand("multiagent-summary", args, 0)
	defer application.Close()
	records, err := application.Store.SummarizeMultiAgentTraces(context.Background(), filters)
	if err != nil {
		log.Fatalf("summarize multiagent traces: %v", err)
	}
	mustWriteJSON(os.Stdout, records)
}

func runMultiAgentVerifierSummary(args []string) {
	filters, application := parseTraceCommand("multiagent-verifiers", args, 0)
	defer application.Close()
	records, err := application.Store.SummarizeMultiAgentVerifierResults(context.Background(), filters)
	if err != nil {
		log.Fatalf("summarize verifier results: %v", err)
	}
	mustWriteJSON(os.Stdout, records)
}

func runMultiAgentTraceFailures(args []string) {
	filters, application := parseTraceCommand("multiagent-failures", args, 50)
	defer application.Close()
	records, err := application.Store.ListMultiAgentTraceFailures(context.Background(), filters)
	if err != nil {
		log.Fatalf("list multiagent failures: %v", err)
	}
	mustWriteJSON(os.Stdout, records)
}

func runMultiAgentTraceHotspots(args []string) {
	filters, application := parseTraceCommand("multiagent-hotspots", args, 50)
	defer application.Close()
	records, err := application.Store.ListMultiAgentTraceHotspots(context.Background(), filters)
	if err != nil {
		log.Fatalf("list multiagent hotspots: %v", err)
	}
	mustWriteJSON(os.Stdout, records)
}

func runMultiAgentReplay(args []string) {
	fs := flag.NewFlagSet("multiagent-replay", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	childSessionID := fs.Int64("child-session-id", 0, "child session id")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if *childSessionID <= 0 {
		log.Fatal("child-session-id is required")
	}
	payload, err := application.ReplayMultiAgentChild(context.Background(), chatUser, *childSessionID)
	if err != nil {
		log.Fatalf("replay multiagent child: %v", err)
	}
	mustWriteJSON(os.Stdout, payload)
}

func runMultiAgentResume(args []string) {
	fs := flag.NewFlagSet("multiagent-resume", flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	childSessionID := fs.Int64("child-session-id", 0, "child session id")
	allowedTools := fs.String("allowed-tools", "", "comma-separated tool allowlist")
	historyWindow := fs.Int("history-window", 0, "history window override")
	_ = fs.Parse(args)

	_, application := mustApp(*configPath)
	defer application.Close()
	chatUser := mustAuthUser(application, strings.TrimSpace(*username), *password, strings.TrimSpace(*token))
	if *childSessionID <= 0 {
		log.Fatal("child-session-id is required")
	}
	payload, err := application.ResumeMultiAgentChild(context.Background(), chatUser, *childSessionID, parseCommaList(*allowedTools), *historyWindow)
	if err != nil {
		log.Fatalf("resume multiagent child: %v", err)
	}
	mustWriteJSON(os.Stdout, payload)
}

func parseTraceCommand(name string, args []string, defaultLimit int) (store.MultiAgentTraceFilters, *app.App) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	configPath := fs.String("config", "./configs/config.example.yaml", "config file")
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	token := fs.String("token", "", "jwt token")
	parentSessionID := fs.Int64("parent-session-id", 0, "parent session id")
	childSessionID := fs.Int64("child-session-id", 0, "child session id")
	taskID := fs.String("task-id", "", "task id")
	from := fs.String("from", "", "start time in RFC3339")
	to := fs.String("to", "", "end time in RFC3339")
	limit := fs.Int("limit", defaultLimit, "record limit")
	offset := fs.Int("offset", 0, "record offset")
	_ = fs.Parse(args)
	return mustTraceFiltersAndApp(*configPath, strings.TrimSpace(*username), *password, strings.TrimSpace(*token), *parentSessionID, *childSessionID, strings.TrimSpace(*taskID), *from, *to, *limit, *offset)
}

func mustTraceFiltersAndApp(configPath, username, password, token string, parentSessionID, childSessionID int64, taskID, from, to string, limit, offset int) (store.MultiAgentTraceFilters, *app.App) {
	_, application := mustApp(configPath)
	chatUser := mustAuthUser(application, username, password, token)
	fromTime, err := parseOptionalTime("from", from)
	if err != nil {
		application.Close()
		log.Fatal(err)
	}
	toTime, err := parseOptionalTime("to", to)
	if err != nil {
		application.Close()
		log.Fatal(err)
	}
	return store.MultiAgentTraceFilters{
		Username:        chatUser,
		ParentSessionID: parentSessionID,
		ChildSessionID:  childSessionID,
		TaskID:          taskID,
		FromTime:        fromTime,
		ToTime:          toTime,
		Limit:           limit,
		Offset:          offset,
	}, application
}

func mustAuthUser(application *app.App, username, password, token string) string {
	chatUser, err := authenticateChatUser(context.Background(), application, username, password, token)
	if err != nil {
		log.Fatalf("auth failed: %v", err)
	}
	return chatUser
}

func buildHistoryPayload(ctx context.Context, st *store.Store, username string, sessionLimit, sessionOffset, messageLimit, messageOffset int) ([]map[string]any, error) {
	sessions, err := st.ListSessionsPage(ctx, username, sessionLimit, sessionOffset)
	if err != nil {
		return nil, err
	}
	history := make([]map[string]any, 0, len(sessions))
	for _, session := range sessions {
		messages, err := st.GetMessagesPage(ctx, session.ID, messageLimit, messageOffset)
		if err != nil {
			return nil, err
		}
		history = append(history, map[string]any{"session": session, "messages": messages})
	}
	return history, nil
}

type executionProfileSummary struct {
	Profile string `json:"profile"`
	Total   int    `json:"total"`
	Success int    `json:"success"`
	Denied  int    `json:"denied"`
}

func buildExecutionProfileAuditPayload(ctx context.Context, st *store.Store, username string, fromTime, toTime time.Time) (map[string]any, error) {
	records, err := st.ListAuditFiltered(ctx, store.AuditFilters{Username: username, Action: "system_exec_profile_%", FromTime: fromTime, ToTime: toTime, Limit: 200})
	if err != nil {
		return nil, err
	}
	actionSummary, err := st.SummarizeAuditActions(ctx, store.AuditFilters{Username: username, Action: "system_exec_profile_%", FromTime: fromTime, ToTime: toTime})
	if err != nil {
		return nil, err
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
	sort.Slice(profileSummary, func(i, j int) bool {
		if profileSummary[i].Denied != profileSummary[j].Denied {
			return profileSummary[i].Denied > profileSummary[j].Denied
		}
		if profileSummary[i].Total != profileSummary[j].Total {
			return profileSummary[i].Total > profileSummary[j].Total
		}
		return profileSummary[i].Profile < profileSummary[j].Profile
	})
	return map[string]any{
		"records":         records,
		"action_summary":  actionSummary,
		"profile_summary": profileSummary,
		"category":        "system_exec_profile",
	}, nil
}

func extractAuditDetailValue(detail, key string) string {
	prefix := key + "="
	for _, field := range strings.Fields(detail) {
		if strings.HasPrefix(field, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(field, prefix))
		}
	}
	return ""
}

func parseOptionalTime(name, raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s time: %w", name, err)
	}
	return parsed, nil
}

func parseOptionalBoolPointer(raw string) *bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		log.Fatalf("invalid completed filter: %v", err)
	}
	return &parsed
}

func parseCommaList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func loadJSONFile(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func loadTasksFile(path string) ([]multiagent.Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tasks []multiagent.Task
	if err := json.Unmarshal(data, &tasks); err == nil {
		return tasks, nil
	}
	var wrapper struct {
		Tasks []multiagent.Task `json:"tasks"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Tasks, nil
}

func loadPlanFile(path string) (multiagent.Plan, error) {
	var plan multiagent.Plan
	if err := loadJSONFile(path, &plan); err != nil {
		return multiagent.Plan{}, err
	}
	return plan, nil
}

func mustWriteJSON(w io.Writer, value any) {
	if err := writePrettyJSON(w, value); err != nil {
		log.Fatalf("encode json: %v", err)
	}
}

func writePrettyJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func authenticateChatUser(ctx context.Context, application *app.App, username, password, token string) (string, error) {
	if token != "" {
		claims, err := application.Auth.ParseToken(token)
		if err != nil {
			return "", err
		}
		return claims.Username, nil
	}
	if username == "" || password == "" {
		return "", fmt.Errorf("token or username/password is required")
	}
	if _, err := application.Auth.Login(ctx, username, password); err != nil {
		return "", err
	}
	return username, nil
}

func readAllFromStdin() string {
	info, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(data)
}

func mustApp(configPath string) (config.Config, *app.App) {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("init app: %v", err)
	}
	return cfg, application
}
