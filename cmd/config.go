package main

import (
	"fmt"
	"os"
	"time"

	"encoding/json"
)

type Config struct {
	OllamaURL     string       `json:"ollama_url"`
	GeminiAPIKey  string       `json:"gemini_api_key"`
	PollInterval  Duration     `json:"poll_interval"`
	MaxIterations int          `json:"max_iterations"`
	MaxCostBudget int          `json:"max_cost_budget"`
	ShadowMode    bool         `json:"shadow_mode"`
	Repos         []RepoConfig `json:"repos"`
}

type RepoConfig struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Token string `json:"token"`
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

	if len(cfg.Repos) == 0 {
		return Config{}, fmt.Errorf("no repos configured")
	}

	for i, r := range cfg.Repos {
		if r.Owner == "" || r.Repo == "" || r.Token == "" {
			return Config{}, fmt.Errorf("repo %d: owner, repo, and token are required", i)
		}
	}

	return cfg, nil
}
