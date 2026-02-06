package dag

import "encoding/json"

// DAG represents a directed acyclic graph containing nodes and edges.
type DAG struct {
	ID    string `json:"id"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node represents a vertex in the DAG.
// Ref is a temporary key used only during CreateDAG for edge wiring — it is never persisted.
type Node struct {
	ID   string          `json:"id,omitempty"`
	Ref  string          `json:"ref,omitempty"`
	Data json.RawMessage `json:"data"`
}

// Edge represents a directed connection between two nodes.
// FromNodeRef / ToNodeRef are temporary keys used only during CreateDAG — they are never persisted.
type Edge struct {
	ID          string          `json:"id,omitempty"`
	FromNodeID  string          `json:"from_node_id,omitempty"`
	ToNodeID    string          `json:"to_node_id,omitempty"`
	FromNodeRef string          `json:"from_node_ref,omitempty"`
	ToNodeRef   string          `json:"to_node_ref,omitempty"`
	Data        json.RawMessage `json:"data"`
}
