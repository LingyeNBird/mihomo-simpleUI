package main

import (
	"log"

	"mihomo-webui-proxy/backend/internal/api"
	"mihomo-webui-proxy/backend/internal/config"
	"mihomo-webui-proxy/backend/internal/service"
	"mihomo-webui-proxy/backend/internal/store"
)

func main() {
	cfg := config.Load()
	if err := cfg.EnsurePaths(); err != nil {
		log.Fatalf("ensure paths: %v", err)
	}
	st, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()
	svc := service.New(cfg, st)
	router := api.NewRouter(svc, cfg.StaticDir)
	log.Printf("starting app on %s", cfg.ListenAddr)
	if err := router.Run(cfg.ListenAddr); err != nil {
		log.Fatalf("run server: %v", err)
	}
}
