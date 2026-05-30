package internal

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ruromero/factory-orchestrator/config"
	"github.com/ruromero/factory-orchestrator/github"
	"github.com/ruromero/factory-orchestrator/pipeline"
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

func NewGitHubClientForApp(cfg config.Config, appRole, owner, repo string) (*github.Client, error) {
	if cfg.Apps != nil {
		if app, ok := cfg.Apps[appRole]; ok && app.AppID != 0 {
			repoCfg, repoOk := FindRepoConfig(cfg, owner, repo)
			installID := app.InstallationID
			if installID == 0 && repoOk {
				installID = repoCfg.InstallationID
			}
			keyPath := app.PrivateKeyPath
			if keyPath == "" && repoOk {
				keyPath = repoCfg.PrivateKeyPath
			}
			if installID != 0 && keyPath != "" {
				auth, err := github.NewAppAuth(app.AppID, keyPath, installID)
				if err != nil {
					return nil, fmt.Errorf("app auth for %s: %w", appRole, err)
				}
				return github.NewClientWithAppAuth(auth, owner, repo), nil
			}
		}
	}
	repoCfg, ok := FindRepoConfig(cfg, owner, repo)
	if !ok {
		return nil, fmt.Errorf("repo %s/%s not found in config and no app %q configured", owner, repo, appRole)
	}
	return NewGitHubClientForRepo(repoCfg)
}

func MustGitHubClientForApp(cfg config.Config, appRole string, state *pipeline.State) *github.Client {
	gh, err := NewGitHubClientForApp(cfg, appRole, state.RepoOwner, state.RepoName)
	if err != nil {
		slog.Error("failed to create github client", "app", appRole, "error", err)
		os.Exit(1)
	}
	return gh
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
