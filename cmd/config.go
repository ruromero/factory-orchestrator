package main

import (
	"fmt"
	"os"
	"time"

	"encoding/json"
)

type Config struct {
	OllamaURL     string        `json:"ollama_url"`
	GeminiAPIKey  string        `json:"gemini_api_key,omitempty"`
	Planner       PlannerConfig `json:"planner"`
	PollInterval  Duration      `json:"poll_interval"`
	MaxIterations int           `json:"max_iterations"`
	MaxCostBudget int           `json:"max_cost_budget"`
	ShadowMode    bool          `json:"shadow_mode"`
	Serena        SerenaConfig  `json:"serena"`
	Repos         []RepoConfig  `json:"repos"`
}

type PlannerConfig struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key,omitempty"`
}

type SerenaConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func (s SerenaConfig) Enabled() bool {
	return s.Command != ""
}

type RepoConfig struct {
	Owner          string `json:"owner"`
	Repo           string `json:"repo"`
	Token          string `json:"token,omitempty"`
	AppID          int64  `json:"app_id,omitempty"`
	PrivateKeyPath string `json:"private_key_path,omitempty"`
	InstallationID int64  `json:"installation_id,omitempty"`
}

func (r RepoConfig) UsesAppAuth() bool {
	return r.AppID != 0 && r.PrivateKeyPath != "" && r.InstallationID != 0
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = dur
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Config{
		OllamaURL:     "http://ollama.ai.svc.cluster.local:11434",
		PollInterval:  Duration{30 * time.Second},
		MaxIterations: 3,
		MaxCostBudget: 100000,
		ShadowMode:    true,
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		cfg.GeminiAPIKey = v
	}

	if v := os.Getenv("PLANNER_API_KEY"); v != "" {
		cfg.Planner.APIKey = v
	}

	if v := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"); v != "" {
		for i := range cfg.Repos {
			if cfg.Repos[i].PrivateKeyPath == "" {
				cfg.Repos[i].PrivateKeyPath = v
			}
		}
	}

	if len(cfg.Repos) == 0 {
		return Config{}, fmt.Errorf("no repos configured")
	}

	for i, r := range cfg.Repos {
		if r.Owner == "" || r.Repo == "" {
			return Config{}, fmt.Errorf("repo %d: owner and repo are required", i)
		}
		if !r.UsesAppAuth() && r.Token == "" {
			return Config{}, fmt.Errorf("repo %d: either token or app auth (app_id, private_key_path, installation_id) required", i)
		}
	}

	return cfg, nil
}
