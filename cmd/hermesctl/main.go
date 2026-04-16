package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"go-hermes-agent/internal/app"
	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/models"
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
	apiKeyEnv := fs.String("api-key-env", "", "direct model API key env")
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
		llmCfg.APIKeyEnv = strings.TrimSpace(*apiKeyEnv)
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
