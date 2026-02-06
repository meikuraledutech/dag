package postgres

import (
	"context"
	"fmt"

	"github.com/dharanigowtham/dag"
	"github.com/google/uuid"
)

// CreateDAG saves a full DAG (nodes + edges) in one transaction.
// Nodes/edges without IDs get auto-generated UUIDs.
// Edge refs (FromNodeRef/ToNodeRef) are resolved to real node IDs.
// Returns the DAG with all IDs filled in.
func (s *PGStore) CreateDAG(ctx context.Context, d *dag.DAG) (*dag.DAG, error) {
	// Build ref → UUID mapping and assign IDs to nodes.
	refMap := make(map[string]string)
	for i := range d.Nodes {
		n := &d.Nodes[i]
		if n.ID == "" {
			n.ID = uuid.NewString()
		}
		if n.Ref != "" {
			refMap[n.Ref] = n.ID
		}
	}

	// Resolve edge refs and assign IDs to edges.
	for i := range d.Edges {
		e := &d.Edges[i]
		if e.ID == "" {
			e.ID = uuid.NewString()
		}
		// Resolve from ref.
		if e.FromNodeRef != "" {
			id, ok := refMap[e.FromNodeRef]
			if !ok {
				return nil, fmt.Errorf("dag: unknown from_node_ref %q", e.FromNodeRef)
			}
			e.FromNodeID = id
		}
		// Resolve to ref.
		if e.ToNodeRef != "" {
			id, ok := refMap[e.ToNodeRef]
			if !ok {
				return nil, fmt.Errorf("dag: unknown to_node_ref %q", e.ToNodeRef)
			}
			e.ToNodeID = id
		}
	}

	// Validate acyclic.
	if err := validateAcyclic(d.Nodes, d.Edges); err != nil {
		return nil, err
	}

	// Persist in a single transaction.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dag: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete existing DAG data if any (replace semantics).
	if _, err := tx.Exec(ctx, `DELETE FROM dag_edges WHERE dag_id = $1`, d.ID); err != nil {
		return nil, fmt.Errorf("dag: delete edges: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dag_nodes WHERE dag_id = $1`, d.ID); err != nil {
		return nil, fmt.Errorf("dag: delete nodes: %w", err)
	}

	// Insert nodes.
	for _, n := range d.Nodes {
		if _, err := tx.Exec(ctx,
			`INSERT INTO dag_nodes (id, dag_id, data) VALUES ($1, $2, $3)`,
			n.ID, d.ID, n.Data,
		); err != nil {
			return nil, fmt.Errorf("dag: insert node %s: %w", n.ID, err)
		}
	}

	// Insert edges.
	for _, e := range d.Edges {
		if _, err := tx.Exec(ctx,
			`INSERT INTO dag_edges (id, dag_id, from_node_id, to_node_id, data) VALUES ($1, $2, $3, $4, $5)`,
			e.ID, d.ID, e.FromNodeID, e.ToNodeID, e.Data,
		); err != nil {
			return nil, fmt.Errorf("dag: insert edge %s: %w", e.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("dag: commit: %w", err)
	}

	// Clear ref fields from response — they are not persisted.
	for i := range d.Nodes {
		d.Nodes[i].Ref = ""
	}
	for i := range d.Edges {
		d.Edges[i].FromNodeRef = ""
		d.Edges[i].ToNodeRef = ""
	}

	return d, nil
}

// GetDAG retrieves a full DAG (nodes + edges) by its ID.
// Returns nil, nil if no nodes exist for the dagID.
func (s *PGStore) GetDAG(ctx context.Context, dagID string) (*dag.DAG, error) {
	d := &dag.DAG{ID: dagID}

	rows, err := s.db.Query(ctx,
		`SELECT id, data FROM dag_nodes WHERE dag_id = $1 ORDER BY created_at`, dagID)
	if err != nil {
		return nil, fmt.Errorf("dag: query nodes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var n dag.Node
		if err := rows.Scan(&n.ID, &n.Data); err != nil {
			return nil, fmt.Errorf("dag: scan node: %w", err)
		}
		d.Nodes = append(d.Nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dag: rows nodes: %w", err)
	}

	if len(d.Nodes) == 0 {
		return nil, nil
	}

	rows, err = s.db.Query(ctx,
		`SELECT id, from_node_id, to_node_id, data FROM dag_edges WHERE dag_id = $1 ORDER BY created_at`, dagID)
	if err != nil {
		return nil, fmt.Errorf("dag: query edges: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e dag.Edge
		if err := rows.Scan(&e.ID, &e.FromNodeID, &e.ToNodeID, &e.Data); err != nil {
			return nil, fmt.Errorf("dag: scan edge: %w", err)
		}
		d.Edges = append(d.Edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dag: rows edges: %w", err)
	}

	return d, nil
}

// DeleteDAG removes all nodes and edges for a dagID.
// No error if the dagID doesn't exist.
func (s *PGStore) DeleteDAG(ctx context.Context, dagID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dag: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM dag_edges WHERE dag_id = $1`, dagID); err != nil {
		return fmt.Errorf("dag: delete edges: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dag_nodes WHERE dag_id = $1`, dagID); err != nil {
		return fmt.Errorf("dag: delete nodes: %w", err)
	}

	return tx.Commit(ctx)
}

// validateAcyclic checks that the edges don't form a cycle using DFS.
func validateAcyclic(nodes []dag.Node, edges []dag.Edge) error {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.FromNodeID] = append(adj[e.FromNodeID], e.ToNodeID)
	}

	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)

	state := make(map[string]int)
	for _, n := range nodes {
		state[n.ID] = unvisited
	}
	// Also include nodes referenced only in edges.
	for _, e := range edges {
		if _, ok := state[e.FromNodeID]; !ok {
			state[e.FromNodeID] = unvisited
		}
		if _, ok := state[e.ToNodeID]; !ok {
			state[e.ToNodeID] = unvisited
		}
	}

	var dfs func(id string) bool
	dfs = func(id string) bool {
		state[id] = visiting
		for _, next := range adj[id] {
			switch state[next] {
			case visiting:
				return true
			case unvisited:
				if dfs(next) {
					return true
				}
			}
		}
		state[id] = visited
		return false
	}

	for id, s := range state {
		if s == unvisited {
			if dfs(id) {
				return dag.ErrCycleDetected
			}
		}
	}

	return nil
}
