package harness

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type lspServer struct {
	marker  string
	name    string
	install func(ctx context.Context, binDir string) error
}

var knownServers = []lspServer{
	{
		marker: "Cargo.toml",
		name:   "rust-analyzer",
		install: func(ctx context.Context, binDir string) error {
			cmd := exec.CommandContext(ctx, "rustup", "component", "add", "rust-analyzer")
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			return cmd.Run()
		},
	},
	{
		marker: "go.mod",
		name:   "gopls",
		install: func(ctx context.Context, binDir string) error {
			cmd := exec.CommandContext(ctx, "go", "install", "golang.org/x/tools/gopls@latest")
			cmd.Env = append(os.Environ(), "GOBIN="+binDir)
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			return cmd.Run()
		},
	},
	{
		marker: "package.json",
		name:   "typescript-language-server",
		install: func(ctx context.Context, binDir string) error {
			cmd := exec.CommandContext(ctx, "npm", "install", "-g",
				"--prefix", binDir,
				"typescript-language-server", "typescript")
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			return cmd.Run()
		},
	},
}

func InstallLanguageServers(ctx context.Context, cloneDir string) (string, error) {
	binDir, err := os.MkdirTemp("", "factory-lsp-*")
	if err != nil {
		return "", fmt.Errorf("create lsp bin dir: %w", err)
	}

	var installed []string
	for _, srv := range knownServers {
		if _, err := os.Stat(filepath.Join(cloneDir, srv.marker)); err != nil {
			continue
		}

		if _, err := exec.LookPath(srv.name); err == nil {
			installed = append(installed, srv.name+" (system)")
			continue
		}

		slog.Info("installing language server", "name", srv.name)
		if err := srv.install(ctx, binDir); err != nil {
			slog.Warn("failed to install language server", "name", srv.name, "error", err)
			continue
		}
		installed = append(installed, srv.name)
	}

	if len(installed) > 0 {
		slog.Info("language servers ready", "servers", strings.Join(installed, ", "))
	}
	return binDir, nil
}
