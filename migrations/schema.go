package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed 001_initial.sql
var Initial string

//go:embed *.sql
var files embed.FS

func Apply(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(71845001)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		data, e := files.ReadFile(entry.Name())
		if e != nil {
			return e
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(data))
		var existing string
		rows, e := tx.Query(ctx, `SELECT checksum FROM schema_migrations WHERE name=$1`, entry.Name())
		if e != nil {
			return e
		}
		found := rows.Next()
		if found {
			e = rows.Scan(&existing)
		}
		rows.Close()
		if e != nil {
			return e
		}
		if found {
			if existing != sum {
				return fmt.Errorf("migration checksum changed: %s", entry.Name())
			}
			continue
		}
		if _, e = tx.Exec(ctx, string(data)); e != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), e)
		}
		if _, e = tx.Exec(ctx, `INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)`, entry.Name(), sum); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
