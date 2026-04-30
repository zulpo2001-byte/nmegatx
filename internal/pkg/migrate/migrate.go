package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	reVersion = regexp.MustCompile(`^(\d+)_.*\.up\.sql$`)
)

type Migration struct {
	Version int64
	Path    string
}

func CollectUpMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var ms []Migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		m := reVersion.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		v, _ := strconv.ParseInt(m[1], 10, 64)
		ms = append(ms, Migration{Version: v, Path: filepath.Join(dir, name)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Version < ms[j].Version })
	return ms, nil
}

func EnsureSchemaMigrations(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (
  version BIGINT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`)
	return err
}

func AppliedVersions(db *sql.DB) (map[int64]bool, error) {
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[int64]bool{}
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func ApplyAll(db *sql.DB, migrationsDir string) (int, error) {
	if err := EnsureSchemaMigrations(db); err != nil {
		return 0, err
	}
	ms, err := CollectUpMigrations(migrationsDir)
	if err != nil {
		return 0, err
	}
	applied, err := AppliedVersions(db)
	if err != nil {
		return 0, err
	}
	appliedCount := 0
	for _, m := range ms {
		if applied[m.Version] {
			continue
		}
		if err := applyOne(db, m); err != nil {
			return appliedCount, err
		}
		appliedCount++
	}
	return appliedCount, nil
}

func applyOne(db *sql.DB, m Migration) error {
	b, err := os.ReadFile(m.Path)
	if err != nil {
		return err
	}
	sqlText := strings.TrimSpace(string(b))
	if sqlText == "" {
		return errors.New("empty migration: " + m.Path)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(sqlText); err != nil {
		return fmt.Errorf("apply %d failed: %w", m.Version, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES($1,$2)`, m.Version, time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

