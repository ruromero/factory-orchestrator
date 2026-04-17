package harness

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const lspCacheDir = "/tmp/factory-lsp"

type lspServer struct {
	marker  string
	name    string
	install func(ctx context.Context, binDir string) error
}

var knownServers = []lspServer{
	{
		marker:  "Cargo.toml",
		name:    "rust-analyzer",
		install: installRustAnalyzer,
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

func installRustAnalyzer(ctx context.Context, binDir string) error {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	}
	url := fmt.Sprintf(
		"https://github.com/rust-lang/rust-analyzer/releases/latest/download/rust-analyzer-%s-unknown-linux-gnu.gz",
		arch,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download rust-analyzer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download rust-analyzer: status %d", resp.StatusCode)
	}

	gzPath := filepath.Join(binDir, "rust-analyzer.gz")
	f, err := os.Create(gzPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("write rust-analyzer.gz: %w", err)
	}
	f.Close()

	outPath := filepath.Join(binDir, "rust-analyzer")
	cmd := exec.CommandContext(ctx, "gunzip", "-f", gzPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gunzip: %w", err)
	}
	return os.Chmod(outPath, 0o755)
}

func InstallLanguageServers(ctx context.Context, cloneDir string) (string, error) {
	if err := os.MkdirAll(lspCacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create lsp cache dir: %w", err)
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

		cached := filepath.Join(lspCacheDir, srv.name)
		npmCached := filepath.Join(lspCacheDir, "bin", srv.name)
		if _, err := os.Stat(cached); err == nil {
			installed = append(installed, srv.name+" (cached)")
			continue
		}
		if _, err := os.Stat(npmCached); err == nil {
			installed = append(installed, srv.name+" (cached)")
			continue
		}

		slog.Info("installing language server", "name", srv.name)
		if err := srv.install(ctx, lspCacheDir); err != nil {
			slog.Warn("failed to install language server", "name", srv.name, "error", err)
			continue
		}
		installed = append(installed, srv.name)
	}

	if len(installed) > 0 {
		slog.Info("language servers ready", "servers", strings.Join(installed, ", "))
	}
	return lspCacheDir, nil
}
