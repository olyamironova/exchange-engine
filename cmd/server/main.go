package main

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/olyamironova/exchange-engine/internal/adapter/cache"
	"github.com/olyamironova/exchange-engine/internal/adapter/pg"
	"github.com/olyamironova/exchange-engine/internal/api/http"
	"github.com/olyamironova/exchange-engine/internal/core"
)

func main() {
	ctx := context.Background()
	pgURL := "postgres://user:password@localhost:5432/exchange_db"
	dbpool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		log.Fatalf("failed to connect to Postgres: %v", err)
	}
	defer dbpool.Close()

	repo := pg.NewRepository(dbpool)

	redisCache := cache.NewRedisCache(
		"localhost:6379",
		"",
		0,
		5*time.Minute,
	)
	engine := core.NewEngine(repo, redisCache)

	server := http.NewHTTPServer(engine)

	addr := ":8080"
	log.Printf("Starting HTTP server on %s...", addr)
	if err := server.Run(addr); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
