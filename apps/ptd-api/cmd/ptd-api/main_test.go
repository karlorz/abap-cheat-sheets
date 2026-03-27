package main

import (
	"io"
	"io/fs"
	"log"
	"testing"
	"testing/fstest"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
)

func TestRunValidationRequiresDSN(t *testing.T) {
	silenceLogs(t)

	if err := validateRunConfig(config.Config{}, true); err == nil {
		t.Fatal("expected validation config error without dsn")
	}
}

func TestValidateRunConfigRequiresAuthForServerMode(t *testing.T) {
	silenceLogs(t)

	err := validateRunConfig(config.Config{
		SQLServerDSN: "sqlserver://user:pass@localhost:1433?database=PTD_READONLY",
	}, false)
	if err == nil {
		t.Fatal("expected auth config error for live sql server mode")
	}
}

func TestValidateRunConfigAllowsValidationWithoutAuth(t *testing.T) {
	silenceLogs(t)

	err := validateRunConfig(config.Config{
		SQLServerDSN: "sqlserver://user:pass@localhost:1433?database=PTD_READONLY",
	}, true)
	if err != nil {
		t.Fatalf("expected validation mode to allow missing auth token, got %v", err)
	}
}

func TestConfigureFilesystemsKeepsConfiguredFS(t *testing.T) {
	silenceLogs(t)

	fsys := fs.FS(fstest.MapFS{
		"contracts/test.json": &fstest.MapFile{Data: []byte(`{}`)},
	})

	cfg := config.Config{
		FS:       fsys,
		RepoRoot: "/tmp/repo",
	}

	if err := configureFilesystems(&cfg); err != nil {
		t.Fatalf("configure filesystems: %v", err)
	}
	if cfg.WebFS != nil {
		t.Fatal("expected web filesystem to stay unset when filesystem is already configured")
	}
	if _, err := fs.Stat(cfg.FS, "contracts/test.json"); err != nil {
		t.Fatalf("expected configured filesystem to remain usable: %v", err)
	}
}

func silenceLogs(t *testing.T) {
	t.Helper()

	previousWriter := log.Writer()
	previousFlags := log.Flags()
	previousPrefix := log.Prefix()

	log.SetOutput(io.Discard)
	log.SetFlags(0)
	log.SetPrefix("")

	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		log.SetPrefix(previousPrefix)
	})
}
