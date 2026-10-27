package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"minimarket/internal/api"
	"minimarket/internal/store"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	connectionString, err := databaseConnectionString()
	if err != nil {
		log.Fatalf("configure PostgreSQL: %v", err)
	}
	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		log.Fatalf("open PostgreSQL connection: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := db.PingContext(ctx); err != nil {
		cancel()
		log.Fatalf("connect to PostgreSQL: %v", err)
	}
	cancel()

	repository := store.New(db)
	if err := repository.Initialize(); err != nil {
		log.Fatalf("initialize PostgreSQL schema: %v", err)
	}
	if legacyFile := os.Getenv("LEGACY_DATA_FILE"); legacyFile != "" {
		imported, err := repository.ImportJSONIfEmpty(legacyFile)
		if err != nil {
			log.Fatalf("import legacy data: %v", err)
		}
		if imported {
			log.Printf("Imported legacy records from %s into PostgreSQL", legacyFile)
		} else {
			log.Printf("PostgreSQL already contains data; skipped legacy import")
		}
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

func databaseConnectionString() (string, error) {
	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		return "", errors.New("POSTGRES_PASSWORD is required")
	}
	host := envOrDefault("POSTGRES_HOST", "localhost")
	port := envOrDefault("POSTGRES_PORT", "5432")
	user := envOrDefault("POSTGRES_USER", "postgres")
	database := envOrDefault("POSTGRES_DB", "postgres")
	sslmode := envOrDefault("POSTGRES_SSLMODE", "disable")

	connection := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}
	query := url.Values{}
	query.Set("sslmode", sslmode)
	connection.RawQuery = query.Encode()
	return connection.String(), nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
