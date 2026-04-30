package main

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"
	"nme-v9/internal/config"
	"nme-v9/internal/pkg/migrate"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	dir := "migrations"
	if v := os.Getenv("MIGRATIONS_DIR"); v != "" {
		dir = v
	}

	db, err := sql.Open("postgres", dsnToURL(cfg.DBDSN))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	n, err := migrate.ApplyAll(db, dir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("migrations applied: %d\n", n)
}

// lib/pq accepts both DSN and URL, but database/sql with pq URL is simplest here.
func dsnToURL(dsn string) string { return dsn }

