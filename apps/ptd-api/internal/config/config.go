package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Addr         string
	RepoRoot     string
	ContractDir  string
	QueryTimeout time.Duration
	SQLServerDSN string
	AuthToken    string
	FS           fs.FS
	WebFS        fs.FS
}

func Load() (Config, error) {
	repoRoot, err := loadRepoRoot()
	if err != nil {
		return Config{}, err
	}

	contractDir := os.Getenv("PTD_CONTRACT_DIR")
	if contractDir == "" {
		contractDir = filepath.Join("files", "dashboard", "contracts")
	}
	if filepath.IsAbs(contractDir) {
		if repoRoot == "" {
			return Config{}, fmt.Errorf("absolute PTD_CONTRACT_DIR requires PTD_REPO_ROOT")
		}

		contractDir, err = filepath.Rel(repoRoot, contractDir)
		if err != nil {
			return Config{}, fmt.Errorf("resolve PTD_CONTRACT_DIR: %w", err)
		}
	}
	contractDir = filepath.ToSlash(filepath.Clean(contractDir))

	timeout := 30 * time.Second
	if raw := os.Getenv("PTD_QUERY_TIMEOUT"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse PTD_QUERY_TIMEOUT: %w", err)
		}
		timeout = parsed
	}

	cfg := Config{
		Addr:         envOr("PTD_ADDR", ":8080"),
		RepoRoot:     repoRoot,
		ContractDir:  contractDir,
		QueryTimeout: timeout,
		SQLServerDSN: os.Getenv("PTD_SQLSERVER_DSN"),
		AuthToken:    os.Getenv("PTD_AUTH_TOKEN"),
	}
	if repoRoot != "" {
		cfg.FS = os.DirFS(repoRoot)
	}

	return cfg, nil
}

func loadRepoRoot() (string, error) {
	if root := os.Getenv("PTD_REPO_ROOT"); root != "" {
		return filepath.Clean(root), nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	root, err := findRepoRoot(wd)
	if err != nil {
		return "", err
	}

	return root, nil
}

func findRepoRoot(start string) (string, error) {
	current := filepath.Clean(start)

	for {
		if isDir(filepath.Join(current, ".git")) {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", nil
		}

		current = parent
	}
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}
