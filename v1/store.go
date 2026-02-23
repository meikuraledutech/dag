package dag

import (
	"context"
	"errors"
)

var (
	ErrCycleDetected = errors.New("dag: cycle detected, graph is not acyclic")
	ErrNodeNotFound  = errors.New("dag: node not found")
	ErrEdgeNotFound  = errors.New("dag: edge not found")
)

// Store defines the contract for persisting and retrieving DAGs.
type Store interface {
	// Schema
	CreateSchema(ctx context.Context) error
	DropSchema(ctx context.Context) error

	// DAG (bulk operations)
	CreateDAG(ctx context.Context, d *DAG) (*DAG, error)
	GetDAG(ctx context.Context, dagID string) (*DAG, error)
	DeleteDAG(ctx context.Context, dagID string) error

	// Nodes
	AddNode(ctx context.Context, dagID string, node *Node) (string, error)
	GetNode(ctx context.Context, nodeID string) (*Node, error)
	UpdateNode(ctx context.Context, node *Node) error
	DeleteNode(ctx context.Context, nodeID string) error
	ListNodes(ctx context.Context, dagID string) ([]Node, error)

	// Edges
	AddEdge(ctx context.Context, dagID string, edge *Edge) (string, error)
	GetEdge(ctx context.Context, edgeID string) (*Edge, error)
	UpdateEdge(ctx context.Context, edge *Edge) error
	DeleteEdge(ctx context.Context, edgeID string) error
	ListEdges(ctx context.Context, dagID string) ([]Edge, error)
}
