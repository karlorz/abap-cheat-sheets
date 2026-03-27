package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/server"
)

//go:embed _embed
var embeddedFiles embed.FS

func main() {
	os.Exit(run())
}

func run() int {
	validateFlag := flag.Bool("validate", false, "run validation checks and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Printf("load config: %v", err)
		return 1
	}

	if err := configureFilesystems(&cfg); err != nil {
		log.Printf("configure filesystems: %v", err)
		return 1
	}

	if err := validateRunConfig(cfg, *validateFlag); err != nil {
		log.Printf("%v", err)
		return 1
	}

	app, err := server.New(cfg)
	if err != nil {
		log.Printf("create server: %v", err)
		return 1
	}
	defer func() {
		if err := app.Close(); err != nil {
			log.Printf("close server resources: %v", err)
		}
	}()

	if *validateFlag {
		return runValidation(app)
	}

	if cfg.SQLServerDSN != "" {
		app.Warmup()
	}

	log.Printf("ptd-api listening on %s", cfg.Addr)
	log.Printf("contract dir: %s", cfg.ContractDir)
	log.Printf("sql server configured: %t", cfg.SQLServerDSN != "")
	log.Printf("auth configured: %t", cfg.AuthToken != "")
	log.Printf("static web configured: %t", cfg.WebFS != nil)

	if err := http.ListenAndServe(cfg.Addr, app.Handler()); err != nil {
		log.Printf("listen and serve: %v", err)
		return 1
	}

	return 0
}

func configureFilesystems(cfg *config.Config) error {
	if cfg.FS == nil {
		var err error
		cfg.FS, err = fs.Sub(embeddedFiles, "_embed")
		if err != nil {
			return fmt.Errorf("load embedded files: %w", err)
		}

		cfg.WebFS, err = fs.Sub(embeddedFiles, "_embed/web")
		if err != nil {
			return fmt.Errorf("load embedded web assets: %w", err)
		}

		log.Printf("repo root: embedded mode")
		return nil
	}

	log.Printf("repo root: %s", cfg.RepoRoot)
	return nil
}

func validateRunConfig(cfg config.Config, validate bool) error {
	if validate {
		if cfg.SQLServerDSN == "" {
			return errors.New("--validate requires PTD_SQLSERVER_DSN")
		}
		return nil
	}

	if cfg.AuthToken == "" && cfg.SQLServerDSN != "" {
		return errors.New("PTD_AUTH_TOKEN is required when PTD_SQLSERVER_DSN is set")
	}

	return nil
}

func runValidation(app *server.App) int {
	report, err := app.Validate(context.Background())
	if err != nil {
		log.Printf("validate: %v", err)
		return 1
	}

	fmt.Print(server.FormatValidationReport(report))
	if report.HasFailures() {
		return 1
	}

	return 0
}
