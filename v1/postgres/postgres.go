package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore implements dag.Store using PostgreSQL via pgx.
type PGStore struct {
	db *pgxpool.Pool
}

// New creates a new PGStore backed by the given pgx connection pool.
func New(db *pgxpool.Pool) *PGStore {
	return &PGStore{db: db}
}
