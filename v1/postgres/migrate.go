package postgres

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	dag "github.com/meikuraledutech/dag/v1"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const createMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS dag_migrations (
	id         SERIAL PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	checksum   TEXT NOT NULL
);`

type migrationFile struct {
	Name     string
	Up       string
	Down     string
	Checksum string
}

type migrationRecord struct {
	ID        int
	Name      string
	AppliedAt time.Time
	Checksum  string
}

// loadMigrations reads migration files from the embedded filesystem, parses them, and sorts by name.
func loadMigrations() ([]migrationFile, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	upFiles := make(map[string]string)
	downFiles := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}

		if strings.HasSuffix(name, ".up.sql") {
			key := strings.TrimSuffix(name, ".up.sql")
			upFiles[key] = string(data)
		} else if strings.HasSuffix(name, ".down.sql") {
			key := strings.TrimSuffix(name, ".down.sql")
			downFiles[key] = string(data)
		}
	}

	var migrations []migrationFile
	for key, up := range upFiles {
		checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(up)))
		migrations = append(migrations, migrationFile{
			Name:     key,
			Up:       up,
			Down:     downFiles[key],
			Checksum: checksum,
		})
	}

	// Sort by name to ensure deterministic order.
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Name < migrations[j].Name
	})

	return migrations, nil
}

// ensureMigrationsTable creates the dag_migrations table if it doesn't exist.
func (s *PGStore) ensureMigrationsTable(ctx context.Context) error {
	_, err := s.db.Exec(ctx, createMigrationsTableSQL)
	return err
}

// appliedMigrations queries all applied migrations from the database.
func (s *PGStore) appliedMigrations(ctx context.Context) (map[string]migrationRecord, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, applied_at, checksum FROM dag_migrations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]migrationRecord)
	for rows.Next() {
		var rec migrationRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.AppliedAt, &rec.Checksum); err != nil {
			return nil, err
		}
		applied[rec.Name] = rec
	}

	return applied, rows.Err()
}

// Migrate applies all pending migrations in order, within transactions.
func (s *PGStore) Migrate(ctx context.Context) error {
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return fmt.Errorf("dag: ensure migrations table: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("dag: load migrations: %w", err)
	}

	applied, err := s.appliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("dag: get applied migrations: %w", err)
	}

	for _, m := range migrations {
		if rec, ok := applied[m.Name]; ok {
			// Already applied. Verify checksum for integrity.
			if rec.Checksum != m.Checksum {
				return fmt.Errorf("dag: migration %s checksum mismatch (expected %s, got %s)", m.Name, rec.Checksum, m.Checksum)
			}
			continue
		}

		// Apply migration in a transaction.
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("dag: begin migration %s: %w", m.Name, err)
		}

		if _, err := tx.Exec(ctx, m.Up); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("dag: run migration %s: %w", m.Name, err)
		}

		if _, err := tx.Exec(ctx, `INSERT INTO dag_migrations (name, checksum) VALUES ($1, $2)`, m.Name, m.Checksum); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("dag: record migration %s: %w", m.Name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("dag: commit migration %s: %w", m.Name, err)
		}
	}

	return nil
}

// Rollback rolls back the last applied migration.
func (s *PGStore) Rollback(ctx context.Context) error {
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return fmt.Errorf("dag: ensure migrations table: %w", err)
	}

	// Get the last applied migration.
	var lastMigration struct {
		ID   int
		Name string
	}

	err := s.db.QueryRow(ctx, `SELECT id, name FROM dag_migrations ORDER BY id DESC LIMIT 1`).
		Scan(&lastMigration.ID, &lastMigration.Name)
	if err != nil {
		return fmt.Errorf("dag: get last migration: %w", err)
	}

	// Load migrations to find the down SQL.
	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("dag: load migrations: %w", err)
	}

	var downSQL string
	for _, m := range migrations {
		if m.Name == lastMigration.Name {
			downSQL = m.Down
			break
		}
	}

	if downSQL == "" {
		return fmt.Errorf("dag: no down migration for %s", lastMigration.Name)
	}

	// Apply rollback in a transaction.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dag: begin rollback %s: %w", lastMigration.Name, err)
	}

	if _, err := tx.Exec(ctx, downSQL); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("dag: run rollback %s: %w", lastMigration.Name, err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM dag_migrations WHERE id = $1`, lastMigration.ID); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("dag: remove migration record %s: %w", lastMigration.Name, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("dag: commit rollback %s: %w", lastMigration.Name, err)
	}

	return nil
}

// MigrationStatus returns all migrations with their applied status.
func (s *PGStore) MigrationStatus(ctx context.Context) ([]dag.MigrationRecord, error) {
	if err := s.ensureMigrationsTable(ctx); err != nil {
		return nil, fmt.Errorf("dag: ensure migrations table: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return nil, fmt.Errorf("dag: load migrations: %w", err)
	}

	applied, err := s.appliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("dag: get applied migrations: %w", err)
	}

	var records []dag.MigrationRecord
	for _, m := range migrations {
		rec := dag.MigrationRecord{
			Name:    m.Name,
			Applied: false,
		}

		if appliedRec, ok := applied[m.Name]; ok {
			rec.Applied = true
			t := appliedRec.AppliedAt
			rec.AppliedAt = &t
			rec.Checksum = appliedRec.Checksum
		}

		records = append(records, rec)
	}

	return records, nil
}
