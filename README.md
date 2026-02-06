# DAG — Directed Acyclic Graph Storage for Go

## Why I Built This

I needed a way to store **conditional forms** — forms where the next question depends on the previous answer. Think of an onboarding flow:

```
"What's your role?"
  ├── "Developer" → "Preferred language?"
  └── "Designer"  → "Preferred design tool?"
```

This is a **DAG (Directed Acyclic Graph)**:
- **Nodes** = questions (or any step/state)
- **Edges** = connections between them (with conditions like "if user picks Developer, go here")

I didn't want to model this as nested JSON or a tree. A DAG is the right data structure — it naturally prevents infinite loops (cycles) and supports branching/merging paths.

So I built this package to:
1. Store DAGs in PostgreSQL (nodes table + edges table)
2. Enforce **no cycles** — if you try to create a loop, it rejects it
3. Let me build the graph **piece by piece** (add one node, add one edge) or **all at once** (bulk insert a whole form)
4. Keep it **interface-based** so I can swap PostgreSQL for another DB later without changing my app code

## What It Does

- **Store entire DAGs** in one shot (bulk create with auto-generated UUIDs)
- **Add/update/delete individual nodes and edges** without touching the rest of the graph
- **Cycle detection** — automatically checks for cycles when adding or updating edges
- **Ref-based wiring** — when creating a DAG in bulk, use temporary "ref" keys to wire nodes together, and the system generates real UUIDs and maps everything
- **Interface-based** — code against `dag.Store`, plug in any backend

## How It's Structured

```
DAG/
├── dag.go              # Types: DAG, Node, Edge (no DB dependency)
├── store.go            # Store interface + error definitions
├── postgres/           # PostgreSQL implementation
│   ├── postgres.go     # PGStore struct, constructor
│   ├── schema.go       # Create/drop tables
│   ├── dag.go          # Bulk DAG operations
│   ├── node.go         # Individual node CRUD
│   └── edge.go         # Individual edge CRUD
├── server/             # Fiber HTTP server (all 16 endpoints)
│   └── main.go
├── example/            # CLI demo
│   └── main.go
├── schema.sql          # Raw SQL for reference
└── DOCS.md             # Complete API reference
```

The key idea: `dag.go` and `store.go` at the root define the **types and interface** with zero database dependency. The `postgres/` package is one implementation. Tomorrow I can add `mysql/` or `sqlite/` without changing a single line in my app.

## Quick Start

```go
import (
    "github.com/meikuraledutech/dag"
    "github.com/meikuraledutech/dag/postgres"
)

// Setup
pool, _ := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
var store dag.Store = postgres.New(pool)
store.CreateSchema(ctx)

// Create a form DAG using refs (no need to manage IDs yourself)
result, _ := store.CreateDAG(ctx, &dag.DAG{
    ID: "onboarding-form",
    Nodes: []dag.Node{
        {Ref: "q1", Data: json.RawMessage(`{"question": "What is your role?"}`)},
        {Ref: "q2", Data: json.RawMessage(`{"question": "Preferred language?"}`)},
        {Ref: "q3", Data: json.RawMessage(`{"question": "Preferred tool?"}`)},
    },
    Edges: []dag.Edge{
        {FromNodeRef: "q1", ToNodeRef: "q2", Data: json.RawMessage(`{"answer": "Developer"}`)},
        {FromNodeRef: "q1", ToNodeRef: "q3", Data: json.RawMessage(`{"answer": "Designer"}`)},
    },
})
// result.Nodes[0].ID → "d959db72-bf20-..." (auto-generated UUID)

// Later, add a new question to the form
q4, _ := store.AddNode(ctx, "onboarding-form", &dag.Node{
    Data: json.RawMessage(`{"question": "Years of experience?"}`),
})

// Connect it (cycle detection runs automatically)
store.AddEdge(ctx, "onboarding-form", &dag.Edge{
    FromNodeID: result.Nodes[1].ID,
    ToNodeID:   q4,
    Data:       json.RawMessage(`{"answer": "any"}`),
})
```

## The Ref System (Why It Exists)

When creating a DAG in bulk, you don't have node IDs yet (they're auto-generated). So how do you tell edges which nodes to connect?

**Refs** solve this. You give each node a temporary name:

```json
{
  "nodes": [
    { "ref": "q1", "data": { "question": "Role?" } },
    { "ref": "q2", "data": { "question": "Language?" } }
  ],
  "edges": [
    { "from_node_ref": "q1", "to_node_ref": "q2", "data": { "answer": "Dev" } }
  ]
}
```

The system:
1. Generates a UUID for each node
2. Maps `"q1"` → `"d959db72-..."`, `"q2"` → `"bf82148f-..."`
3. Resolves edge refs to real IDs
4. Returns everything with real IDs, refs stripped

Refs are never stored in the database. They only exist during the CreateDAG call.

## HTTP Server

The `server/` directory has a ready-to-run Fiber v3 server with all 16 endpoints:

```bash
export DATABASE_URL='postgresql://...'
go run ./server/
```

```
POST   /schema              Create tables
DELETE /schema              Drop tables

POST   /dag                 Create full DAG (bulk)
GET    /dag/:id             Get full DAG
DELETE /dag/:id             Delete full DAG

POST   /dag/:id/nodes       Add a node
GET    /dag/:id/nodes       List all nodes
GET    /nodes/:id           Get a node
PUT    /nodes/:id           Update a node
DELETE /nodes/:id           Delete a node (cascades edges)

POST   /dag/:id/edges       Add an edge (with cycle check)
GET    /dag/:id/edges       List all edges
GET    /edges/:id           Get an edge
PUT    /edges/:id           Update an edge (with cycle check)
DELETE /edges/:id           Delete an edge
```

## Error Handling

Three sentinel errors you can check with `errors.Is()`:

```go
dag.ErrCycleDetected  // tried to create a cycle
dag.ErrNodeNotFound   // UpdateNode on non-existent ID
dag.ErrEdgeNotFound   // UpdateEdge on non-existent ID
```

## Use Cases

This isn't just for forms. A DAG can model:

- **Conditional forms / surveys** — next question depends on the answer
- **Workflow engines** — step A must complete before step B
- **Task dependencies** — build systems, CI/CD pipelines
- **Decision trees** — if-then-else logic stored as data
- **Course prerequisites** — subject A requires subject B first

Anything where you have steps/states with directed connections and need to guarantee no infinite loops.

## Documentation

See [DOCS.md](DOCS.md) for the complete API reference with:
- Every method's input/output JSON
- All error scenarios
- HTTP status code mapping
- curl examples for every endpoint
- Migration commands
- Fiber integration patterns

## Requirements

- Go 1.25+
- PostgreSQL (tested with Neon)
- `github.com/jackc/pgx/v5` (PostgreSQL driver)
- `github.com/google/uuid` (ID generation)
- `github.com/gofiber/fiber/v3` (HTTP server, optional)

## License

BSD 3-Clause License. See [LICENSE](LICENSE) for details.
