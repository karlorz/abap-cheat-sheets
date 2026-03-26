package main

import (
	"log"
	"net/http"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	app, err := server.New(cfg)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	log.Printf("ptd-api listening on %s", cfg.Addr)
	log.Printf("repo root: %s", cfg.RepoRoot)
	log.Printf("contract dir: %s", cfg.ContractDir)
	log.Printf("sql server configured: %t", cfg.SQLServerDSN != "")

	if err := http.ListenAndServe(cfg.Addr, app.Handler()); err != nil {
		log.Fatalf("listen and serve: %v", err)
	}
}
