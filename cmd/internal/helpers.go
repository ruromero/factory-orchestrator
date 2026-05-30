package internal

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ruromero/la-fabriquilla/config"
	"github.com/ruromero/la-fabriquilla/github"
	"github.com/ruromero/la-fabriquilla/pipeline"
)

func MustLoadConfigAndState() (config.Config, *pipeline.State) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.json"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	statePath := os.Getenv("PIPELINE_STATE_PATH")
	if statePath == "" {
		slog.Error("PIPELINE_STATE_PATH not set")
		os.Exit(1)
	}
	state, err := pipeline.LoadState(statePath)
	if err != nil {
		slog.Error("failed to load state", "error", err)
		os.Exit(1)
	}
	return cfg, state
}

func MustSaveState(state *pipeline.State) {
	statePath := os.Getenv("PIPELINE_STATE_PATH")
	if err := pipeline.SaveState(statePath, state); err != nil {
		slog.Error("failed to save state", "error", err)
		os.Exit(1)
	}
}

func NewGitHubClientForRepo(repo config.RepoConfig) (*github.Client, error) {
	if repo.UsesAppAuth() {
		auth, err := github.NewAppAuth(repo.AppID, repo.PrivateKeyPath, repo.InstallationID)
		if err != nil {
			return nil, fmt.Errorf("app auth: %w", err)
		}
		return github.NewClientWithAppAuth(auth, repo.Owner, repo.Repo), nil
	}
	return github.NewClient(repo.Token, repo.Owner, repo.Repo), nil
}

func FindRepoConfig(cfg config.Config, owner, repo string) (config.RepoConfig, bool) {
	for _, r := range cfg.Repos {
		if r.Owner == owner && r.Repo == repo {
			return r, true
		}
	}
	return config.RepoConfig{}, false
}

func MustGitHubClient(cfg config.Config, state *pipeline.State) *github.Client {
	repoCfg, ok := FindRepoConfig(cfg, state.RepoOwner, state.RepoName)
	if !ok {
		slog.Error("repo not found in config", "owner", state.RepoOwner, "repo", state.RepoName)
		os.Exit(1)
	}
	gh, err := NewGitHubClientForRepo(repoCfg)
	if err != nil {
		slog.Error("failed to create github client", "error", err)
		os.Exit(1)
	}
	return gh
}
