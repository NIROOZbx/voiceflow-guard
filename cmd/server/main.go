package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/config"
	"github.com/NIROOZbx/voiceflow-guard/internal/database"
	"github.com/NIROOZbx/voiceflow-guard/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	

	if cfg.DatabaseURL == "" {
		log.Println("DATABASE_URL is empty; starting without a database connection")
	}

	db, err := database.Connect(context.Background(),cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("connect to database: %v", err)
		}

	router := server.NewRouter()
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := runHTTPServer(httpServer,db); err != nil {
		log.Fatalf("HTTP server: %v", err)
	}
}
