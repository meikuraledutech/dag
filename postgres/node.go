package postgres

import (
	"context"
	"fmt"

	"github.com/dharanigowtham/dag"
	"github.com/google/uuid"
)

// AddNode inserts a single node into a DAG.
// If node.ID is empty, a UUID is auto-generated.
// Returns the node ID (generated or provided).
func (s *PGStore) AddNode(ctx context.Context, dagID string, node *dag.Node) (string, error) {
	if node.ID == "" {
		node.ID = uuid.NewString()
	}

	_, err := s.db.Exec(ctx,
		`INSERT INTO dag_nodes (id, dag_id, data) VALUES ($1, $2, $3)`,
		node.ID, dagID, node.Data,
	)
	if err != nil {
		return "", fmt.Errorf("dag: insert node: %w", err)
	}

	return node.ID, nil
}

// GetNode fetches a single node by its ID.
// Returns nil, nil if not found.
func (s *PGStore) GetNode(ctx context.Context, nodeID string) (*dag.Node, error) {
	var n dag.Node
	err := s.db.QueryRow(ctx,
		`SELECT id, data FROM dag_nodes WHERE id = $1`, nodeID,
	).Scan(&n.ID, &n.Data)

	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("dag: get node: %w", err)
	}

	return &n, nil
}

// UpdateNode updates the data of an existing node.
// Returns ErrNodeNotFound if the node doesn't exist.
func (s *PGStore) UpdateNode(ctx context.Context, node *dag.Node) error {
	ct, err := s.db.Exec(ctx,
		`UPDATE dag_nodes SET data = $1 WHERE id = $2`,
		node.Data, node.ID,
	)
	if err != nil {
		return fmt.Errorf("dag: update node: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return dag.ErrNodeNotFound
	}
	return nil
}

// DeleteNode deletes a node by its ID.
// Associated edges are cascade-deleted by the DB.
// No error if the node doesn't exist.
func (s *PGStore) DeleteNode(ctx context.Context, nodeID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM dag_nodes WHERE id = $1`, nodeID)
	if err != nil {
		return fmt.Errorf("dag: delete node: %w", err)
	}
	return nil
}

// ListNodes returns all nodes for a dagID, ordered by created_at.
// Returns an empty slice (not nil) if none found.
func (s *PGStore) ListNodes(ctx context.Context, dagID string) ([]dag.Node, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, data FROM dag_nodes WHERE dag_id = $1 ORDER BY created_at`, dagID)
	if err != nil {
		return nil, fmt.Errorf("dag: list nodes: %w", err)
	}
	defer rows.Close()

	nodes := []dag.Node{}
	for rows.Next() {
		var n dag.Node
		if err := rows.Scan(&n.ID, &n.Data); err != nil {
			return nil, fmt.Errorf("dag: scan node: %w", err)
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dag: rows nodes: %w", err)
	}

	return nodes, nil
}

// isNoRows checks if the error is a "no rows" error from pgx.
func isNoRows(err error) bool {
	return err != nil && err.Error() == "no rows in result set"
}
