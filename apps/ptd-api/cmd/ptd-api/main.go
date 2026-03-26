package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/server"
)

//go:embed _embed
var embeddedFiles embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.AuthToken == "" && cfg.SQLServerDSN != "" {
		log.Fatalf("PTD_AUTH_TOKEN is required when PTD_SQLSERVER_DSN is set")
	}

	if cfg.FS == nil {
		cfg.FS, err = fs.Sub(embeddedFiles, "_embed")
		if err != nil {
			log.Fatalf("load embedded files: %v", err)
		}

		cfg.WebFS, err = fs.Sub(embeddedFiles, "_embed/web")
		if err != nil {
			log.Fatalf("load embedded web assets: %v", err)
		}

		log.Printf("repo root: embedded mode")
	} else {
		log.Printf("repo root: %s", cfg.RepoRoot)
	}

	app, err := server.New(cfg)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}
	defer func() {
		if err := app.Close(); err != nil {
			log.Printf("close server resources: %v", err)
		}
	}()

	log.Printf("ptd-api listening on %s", cfg.Addr)
	log.Printf("contract dir: %s", cfg.ContractDir)
	log.Printf("sql server configured: %t", cfg.SQLServerDSN != "")
	log.Printf("auth configured: %t", cfg.AuthToken != "")
	log.Printf("static web configured: %t", cfg.WebFS != nil)

	if err := http.ListenAndServe(cfg.Addr, app.Handler()); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}
