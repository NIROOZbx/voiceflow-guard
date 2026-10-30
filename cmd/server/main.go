package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/config"
	"github.com/NIROOZbx/voiceflow-guard/internal/database"
	"github.com/NIROOZbx/voiceflow-guard/internal/database/sqlc"
	"github.com/NIROOZbx/voiceflow-guard/internal/repository"
	"github.com/NIROOZbx/voiceflow-guard/internal/server"
	"github.com/NIROOZbx/voiceflow-guard/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load configuration: %v", err)
	}

	log.Println("connecting to database...")
	db, err := database.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	log.Println("database connection established successfully")

	queries := sqlc.New(db)

	operationRepo := repository.NewOpsRepo(queries)
	repository.NewBookingsRepo(queries)

	txManager := repository.NewTransactionManager(db)
	opsService := service.NewOpsService(operationRepo,txManager)
	bookingHandler := server.NewBookingHandler(opsService)

	router := server.NewRouter(bookingHandler)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if err := runHTTPServer(httpServer, db); err != nil {
		log.Fatalf("HTTP server: %v", err)
	}
}
