package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"minimarket/internal/api"
	"minimarket/internal/store"
)

func main() {
	dataFile := os.Getenv("DATA_FILE")
	if dataFile == "" {
		dataFile = "data/store.json"
	}

	repository := store.New(dataFile)
	if err := repository.Check(); err != nil {
		log.Fatalf("load inventory data: %v", err)
	}

	address := os.Getenv("API_ADDRESS")
	if address == "" {
		address = ":8080"
	}

	server := &http.Server{
		Addr:              address,
		Handler:           api.New(repository),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("Minimarket API listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("start API server: %v", err)
	}
}
