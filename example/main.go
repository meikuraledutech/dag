package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/meikuraledutech/dag"
	"github.com/meikuraledutech/dag/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// Wire up the postgres implementation behind the Store interface.
	var store dag.Store = postgres.New(pool)

	// 1. Create tables
	if err := store.CreateSchema(ctx); err != nil {
		log.Fatalf("schema: %v", err)
	}
	fmt.Println("schema created")

	// ── Bulk insert using refs (tree insert) ──────────────────────────
	form := &dag.DAG{
		ID: "onboarding-form",
		Nodes: []dag.Node{
			{Ref: "q1", Data: json.RawMessage(`{"question": "What is your role?", "type": "select"}`)},
			{Ref: "q2", Data: json.RawMessage(`{"question": "Preferred language?", "type": "select"}`)},
			{Ref: "q3", Data: json.RawMessage(`{"question": "Preferred design tool?", "type": "select"}`)},
		},
		Edges: []dag.Edge{
			{FromNodeRef: "q1", ToNodeRef: "q2", Data: json.RawMessage(`{"answer": "Developer"}`)},
			{FromNodeRef: "q1", ToNodeRef: "q3", Data: json.RawMessage(`{"answer": "Designer"}`)},
		},
	}

	created, err := store.CreateDAG(ctx, form)
	if err != nil {
		log.Fatalf("create dag: %v", err)
	}
	fmt.Println("dag created (bulk with refs)")
	printJSON(created)

	// ── Retrieve ──────────────────────────────────────────────────────
	result, err := store.GetDAG(ctx, "onboarding-form")
	if err != nil {
		log.Fatalf("get dag: %v", err)
	}
	fmt.Println("\ndag retrieved:")
	printJSON(result)

	// ── Granular: add a single node ───────────────────────────────────
	q4ID, err := store.AddNode(ctx, "onboarding-form", &dag.Node{
		Data: json.RawMessage(`{"question": "Years of experience?", "type": "number"}`),
	})
	if err != nil {
		log.Fatalf("add node: %v", err)
	}
	fmt.Printf("\nadded node: %s\n", q4ID)

	// ── Granular: add an edge from q2 → q4 ────────────────────────────
	q2ID := created.Nodes[1].ID
	edgeID, err := store.AddEdge(ctx, "onboarding-form", &dag.Edge{
		FromNodeID: q2ID,
		ToNodeID:   q4ID,
		Data:       json.RawMessage(`{"answer": "any"}`),
	})
	if err != nil {
		log.Fatalf("add edge: %v", err)
	}
	fmt.Printf("added edge: %s\n", edgeID)

	// ── List nodes ────────────────────────────────────────────────────
	nodes, err := store.ListNodes(ctx, "onboarding-form")
	if err != nil {
		log.Fatalf("list nodes: %v", err)
	}
	fmt.Printf("\nnodes (%d):\n", len(nodes))
	printJSON(nodes)

	// ── Cleanup ───────────────────────────────────────────────────────
	if err := store.DeleteDAG(ctx, "onboarding-form"); err != nil {
		log.Fatalf("delete: %v", err)
	}
	fmt.Println("\ndag deleted")
}

func printJSON(v any) {
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
}
