package postgres

import (
	"context"
	"fmt"

	"github.com/meikuraledutech/dag"
	"github.com/google/uuid"
)

// AddEdge inserts a single edge into a DAG.
// If edge.ID is empty, a UUID is auto-generated.
// Validates that adding this edge does not create a cycle.
// Returns the edge ID (generated or provided).
func (s *PGStore) AddEdge(ctx context.Context, dagID string, edge *dag.Edge) (string, error) {
	if edge.ID == "" {
		edge.ID = uuid.NewString()
	}

	// Fetch existing edges + nodes for cycle detection.
	nodes, err := s.ListNodes(ctx, dagID)
	if err != nil {
		return "", err
	}
	edges, err := s.ListEdges(ctx, dagID)
	if err != nil {
		return "", err
	}

	// Append the new edge and validate.
	edges = append(edges, *edge)
	if err := validateAcyclic(nodes, edges); err != nil {
		return "", err
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO dag_edges (id, dag_id, from_node_id, to_node_id, data) VALUES ($1, $2, $3, $4, $5)`,
		edge.ID, dagID, edge.FromNodeID, edge.ToNodeID, edge.Data,
	)
	if err != nil {
		return "", fmt.Errorf("dag: insert edge: %w", err)
	}

	return edge.ID, nil
}

// GetEdge fetches a single edge by its ID.
// Returns nil, nil if not found.
func (s *PGStore) GetEdge(ctx context.Context, edgeID string) (*dag.Edge, error) {
	var e dag.Edge
	err := s.db.QueryRow(ctx,
		`SELECT id, from_node_id, to_node_id, data FROM dag_edges WHERE id = $1`, edgeID,
	).Scan(&e.ID, &e.FromNodeID, &e.ToNodeID, &e.Data)

	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("dag: get edge: %w", err)
	}

	return &e, nil
}

// UpdateEdge updates an existing edge's from_node_id, to_node_id, and data.
// Validates that the update does not create a cycle.
// Returns ErrEdgeNotFound if the edge doesn't exist.
func (s *PGStore) UpdateEdge(ctx context.Context, edge *dag.Edge) error {
	// First find the edge's dag_id.
	var dagID string
	err := s.db.QueryRow(ctx,
		`SELECT dag_id FROM dag_edges WHERE id = $1`, edge.ID,
	).Scan(&dagID)
	if err != nil {
		if isNoRows(err) {
			return dag.ErrEdgeNotFound
		}
		return fmt.Errorf("dag: find edge: %w", err)
	}

	// Fetch existing data for cycle detection.
	nodes, err := s.ListNodes(ctx, dagID)
	if err != nil {
		return err
	}
	existingEdges, err := s.ListEdges(ctx, dagID)
	if err != nil {
		return err
	}

	// Replace the updated edge in the list.
	for i, e := range existingEdges {
		if e.ID == edge.ID {
			existingEdges[i].FromNodeID = edge.FromNodeID
			existingEdges[i].ToNodeID = edge.ToNodeID
			break
		}
	}

	if err := validateAcyclic(nodes, existingEdges); err != nil {
		return err
	}

	ct, err := s.db.Exec(ctx,
		`UPDATE dag_edges SET from_node_id = $1, to_node_id = $2, data = $3 WHERE id = $4`,
		edge.FromNodeID, edge.ToNodeID, edge.Data, edge.ID,
	)
	if err != nil {
		return fmt.Errorf("dag: update edge: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return dag.ErrEdgeNotFound
	}
	return nil
}

// DeleteEdge deletes an edge by its ID.
// No error if the edge doesn't exist.
func (s *PGStore) DeleteEdge(ctx context.Context, edgeID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM dag_edges WHERE id = $1`, edgeID)
	if err != nil {
		return fmt.Errorf("dag: delete edge: %w", err)
	}
	return nil
}

// ListEdges returns all edges for a dagID, ordered by created_at.
// Returns an empty slice (not nil) if none found.
func (s *PGStore) ListEdges(ctx context.Context, dagID string) ([]dag.Edge, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, from_node_id, to_node_id, data FROM dag_edges WHERE dag_id = $1 ORDER BY created_at`, dagID)
	if err != nil {
		return nil, fmt.Errorf("dag: list edges: %w", err)
	}
	defer rows.Close()

	edges := []dag.Edge{}
	for rows.Next() {
		var e dag.Edge
		if err := rows.Scan(&e.ID, &e.FromNodeID, &e.ToNodeID, &e.Data); err != nil {
			return nil, fmt.Errorf("dag: scan edge: %w", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dag: rows edges: %w", err)
	}

	return edges, nil
}
