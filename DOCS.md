# DAG Package — Complete Reference

Interface-based DAG (Directed Acyclic Graph) storage. Code against `dag.Store`, wire up any backend (`postgres/`, future `mysql/`, `sqlite/`, etc.).

---

## Table of Contents

1. [Installation & Setup](#installation--setup)
2. [Database Schema](#database-schema)
3. [Types](#types)
4. [Sentinel Errors](#sentinel-errors)
5. [Store Interface](#store-interface)
6. [Schema Operations](#schema-operations)
7. [DAG Operations (Bulk)](#dag-operations-bulk)
   - [CreateDAG](#createdag)
   - [GetDAG](#getdag)
   - [DeleteDAG](#deletedag)
8. [Node Operations (Granular)](#node-operations-granular)
   - [AddNode](#addnode)
   - [GetNode](#getnode)
   - [UpdateNode](#updatenode)
   - [DeleteNode](#deletenode)
   - [ListNodes](#listnodes)
9. [Edge Operations (Granular)](#edge-operations-granular)
   - [AddEdge](#addedge)
   - [GetEdge](#getedge)
   - [UpdateEdge](#updateedge)
   - [DeleteEdge](#deleteedge)
   - [ListEdges](#listedges)
10. [ID Generation Rules](#id-generation-rules)
11. [Cycle Detection](#cycle-detection)
12. [Error Handling Guide](#error-handling-guide)
13. [HTTP Status Code Mapping](#http-status-code-mapping)
14. [Migration & Schema Management](#migration--schema-management)
15. [Fiber Integration (Full Example)](#fiber-integration-full-example)

---

## Installation & Setup

```bash
go get github.com/meikuraledutech/dag
```

```go
import (
    "github.com/meikuraledutech/dag"
    "github.com/meikuraledutech/dag/postgres"
)
```

### Initialize

```go
pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
if err != nil {
    log.Fatal(err)
}

var store dag.Store = postgres.New(pool)

// Create tables on startup (idempotent, safe to call every time)
if err := store.CreateSchema(ctx); err != nil {
    log.Fatal(err)
}
```

### Folder Structure

```
DAG/
├── dag.go              # Types: DAG, Node, Edge
├── store.go            # Store interface + sentinel errors
├── postgres/
│   ├── postgres.go     # PGStore struct, New()
│   ├── schema.go       # CreateSchema, DropSchema
│   ├── dag.go          # CreateDAG, GetDAG, DeleteDAG
│   ├── node.go         # AddNode, GetNode, UpdateNode, DeleteNode, ListNodes
│   └── edge.go         # AddEdge, GetEdge, UpdateEdge, DeleteEdge, ListEdges
├── schema.sql          # Raw SQL reference
├── server/
│   └── main.go         # Fiber HTTP server
└── example/
    └── main.go         # CLI demo
```

---

## Database Schema

Two tables. No separate DAG table — a DAG is a logical grouping by `dag_id`.

```sql
CREATE TABLE IF NOT EXISTS dag_nodes (
    id         TEXT PRIMARY KEY,
    dag_id     TEXT NOT NULL,
    data       JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dag_edges (
    id           TEXT PRIMARY KEY,
    dag_id       TEXT NOT NULL,
    from_node_id TEXT NOT NULL REFERENCES dag_nodes(id) ON DELETE CASCADE,
    to_node_id   TEXT NOT NULL REFERENCES dag_nodes(id) ON DELETE CASCADE,
    data         JSONB NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dag_nodes_dag_id ON dag_nodes(dag_id);
CREATE INDEX IF NOT EXISTS idx_dag_edges_dag_id ON dag_edges(dag_id);
CREATE INDEX IF NOT EXISTS idx_dag_edges_from   ON dag_edges(from_node_id);
CREATE INDEX IF NOT EXISTS idx_dag_edges_to     ON dag_edges(to_node_id);
```

**Key points:**
- `dag_id` groups nodes/edges into logical DAGs
- `ON DELETE CASCADE` on edges — deleting a node auto-deletes its edges
- `data` is JSONB — store any JSON structure (questions, metadata, config)
- `created_at` used for ordering in List/Get operations

---

## Types

### DAG

```go
type DAG struct {
    ID    string `json:"id"`
    Nodes []Node `json:"nodes"`
    Edges []Edge `json:"edges"`
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | Yes | Unique identifier for the DAG |
| `nodes` | `[]Node` | Yes | List of nodes in the DAG |
| `edges` | `[]Edge` | No | List of edges connecting nodes |

### Node

```go
type Node struct {
    ID   string          `json:"id,omitempty"`
    Ref  string          `json:"ref,omitempty"`
    Data json.RawMessage `json:"data"`
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | No | Unique node ID. Auto-generated UUID if empty. |
| `ref` | `string` | No | Temporary key for CreateDAG edge wiring. **Never persisted.** |
| `data` | `json.RawMessage` | Yes | Arbitrary JSON payload (question, metadata, etc.) |

### Edge

```go
type Edge struct {
    ID          string          `json:"id,omitempty"`
    FromNodeID  string          `json:"from_node_id,omitempty"`
    ToNodeID    string          `json:"to_node_id,omitempty"`
    FromNodeRef string          `json:"from_node_ref,omitempty"`
    ToNodeRef   string          `json:"to_node_ref,omitempty"`
    Data        json.RawMessage `json:"data"`
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | `string` | No | Unique edge ID. Auto-generated UUID if empty. |
| `from_node_id` | `string` | Conditional | Source node ID. Required for AddEdge/UpdateEdge. |
| `to_node_id` | `string` | Conditional | Target node ID. Required for AddEdge/UpdateEdge. |
| `from_node_ref` | `string` | Conditional | Temp ref to source node. **Only for CreateDAG. Never persisted.** |
| `to_node_ref` | `string` | Conditional | Temp ref to target node. **Only for CreateDAG. Never persisted.** |
| `data` | `json.RawMessage` | Yes | Arbitrary JSON payload (answer condition, weight, etc.) |

---

## Sentinel Errors

```go
dag.ErrCycleDetected  // "dag: cycle detected, graph is not acyclic"
dag.ErrNodeNotFound   // "dag: node not found"
dag.ErrEdgeNotFound   // "dag: edge not found"
```

Check with `errors.Is()`:
```go
if errors.Is(err, dag.ErrCycleDetected) { ... }
if errors.Is(err, dag.ErrNodeNotFound)  { ... }
if errors.Is(err, dag.ErrEdgeNotFound)  { ... }
```

---

## Store Interface

```go
type Store interface {
    CreateSchema(ctx context.Context) error
    DropSchema(ctx context.Context) error

    CreateDAG(ctx context.Context, d *DAG) (*DAG, error)
    GetDAG(ctx context.Context, dagID string) (*DAG, error)
    DeleteDAG(ctx context.Context, dagID string) error

    AddNode(ctx context.Context, dagID string, node *Node) (string, error)
    GetNode(ctx context.Context, nodeID string) (*Node, error)
    UpdateNode(ctx context.Context, node *Node) error
    DeleteNode(ctx context.Context, nodeID string) error
    ListNodes(ctx context.Context, dagID string) ([]Node, error)

    AddEdge(ctx context.Context, dagID string, edge *Edge) (string, error)
    GetEdge(ctx context.Context, edgeID string) (*Edge, error)
    UpdateEdge(ctx context.Context, edge *Edge) error
    DeleteEdge(ctx context.Context, edgeID string) error
    ListEdges(ctx context.Context, dagID string) ([]Edge, error)
}
```

---

## Schema Operations

### CreateSchema

```
CreateSchema(ctx context.Context) error
```

Creates `dag_nodes` and `dag_edges` tables with indexes. **Idempotent** — uses `IF NOT EXISTS`, safe to call on every app startup.

| Scenario | Returns |
|----------|---------|
| Tables created or already exist | `nil` |
| DB connection failed | `error` (wrapped DB error) |

```go
err := store.CreateSchema(ctx)
```

### DropSchema

```
DropSchema(ctx context.Context) error
```

Drops both tables with `CASCADE`. **Idempotent** — uses `IF EXISTS`. **Destructive — all data is lost.**

| Scenario | Returns |
|----------|---------|
| Tables dropped or didn't exist | `nil` |
| DB connection failed | `error` (wrapped DB error) |

```go
err := store.DropSchema(ctx)
```

---

## DAG Operations (Bulk)

### CreateDAG

```
CreateDAG(ctx context.Context, d *DAG) (*DAG, error)
```

Saves a full DAG (nodes + edges) in **one transaction**. Returns the DAG with all generated IDs filled in. Ref fields are stripped from the response.

**Behavior:**
- Validates acyclic before inserting
- If a DAG with the same ID already exists, it is **replaced** (delete + re-insert in same tx)
- All UUIDs are generated by the app (not the DB)

#### Mode 1: Using refs (no IDs — full auto-generation)

Nodes have `ref` as temp keys. Edges reference nodes via `from_node_ref` / `to_node_ref`. All real IDs are auto-generated UUIDs.

**Input:**
```json
{
  "id": "onboarding-form",
  "nodes": [
    { "ref": "q1", "data": { "question": "What is your role?", "type": "select" } },
    { "ref": "q2", "data": { "question": "Preferred language?", "type": "select" } },
    { "ref": "q3", "data": { "question": "Preferred tool?", "type": "select" } }
  ],
  "edges": [
    { "from_node_ref": "q1", "to_node_ref": "q2", "data": { "answer": "Developer" } },
    { "from_node_ref": "q1", "to_node_ref": "q3", "data": { "answer": "Designer" } }
  ]
}
```

**Output (201):**
```json
{
  "id": "onboarding-form",
  "nodes": [
    { "id": "d959db72-bf20-4d96-9b39-4c2c5371c355", "data": { "question": "What is your role?", "type": "select" } },
    { "id": "bf82148f-fbfd-450e-b371-bbcd726624ac", "data": { "question": "Preferred language?", "type": "select" } },
    { "id": "ab54f559-fbd2-4969-a050-6386fd9c01af", "data": { "question": "Preferred tool?", "type": "select" } }
  ],
  "edges": [
    { "id": "de054cf9-7406-41f6-aa9a-e3926ab32ea4", "from_node_id": "d959db72-bf20-4d96-9b39-4c2c5371c355", "to_node_id": "bf82148f-fbfd-450e-b371-bbcd726624ac", "data": { "answer": "Developer" } },
    { "id": "a1c4e839-f2c1-4d8a-bdf0-33076b77429f", "from_node_id": "d959db72-bf20-4d96-9b39-4c2c5371c355", "to_node_id": "ab54f559-fbd2-4969-a050-6386fd9c01af", "data": { "answer": "Designer" } }
  ]
}
```

**What happened internally:**
```
ref "q1" → UUID "d959db72-..."
ref "q2" → UUID "bf82148f-..."
ref "q3" → UUID "ab54f559-..."
edge from_node_ref "q1" → resolved to "d959db72-..."
edge to_node_ref "q2"   → resolved to "bf82148f-..."
```

#### Mode 2: With explicit IDs

Caller provides all IDs. Edges use `from_node_id` / `to_node_id` directly.

**Input:**
```json
{
  "id": "form-1",
  "nodes": [
    { "id": "q1", "data": { "question": "Name?" } },
    { "id": "q2", "data": { "question": "Age?" } }
  ],
  "edges": [
    { "id": "e1", "from_node_id": "q1", "to_node_id": "q2", "data": { "answer": "next" } }
  ]
}
```

**Output (201):** Same structure, IDs unchanged.

#### Mode 3: Mixed (some IDs, some refs)

**Input:**
```json
{
  "id": "form-1",
  "nodes": [
    { "id": "q1", "data": { "question": "Name?" } },
    { "ref": "temp", "data": { "question": "Age?" } }
  ],
  "edges": [
    { "from_node_id": "q1", "to_node_ref": "temp", "data": { "answer": "next" } }
  ]
}
```

- Node with `id` → kept as-is
- Node with `ref` → UUID generated
- Edge can mix `from_node_id` (real) with `to_node_ref` (temp)

**Output (201):**
```json
{
  "id": "form-1",
  "nodes": [
    { "id": "q1", "data": { "question": "Name?" } },
    { "id": "x4y5z6-...", "data": { "question": "Age?" } }
  ],
  "edges": [
    { "id": "a1b2c3-...", "from_node_id": "q1", "to_node_id": "x4y5z6-...", "data": { "answer": "next" } }
  ]
}
```

#### Error scenarios

| Scenario | Error | HTTP |
|----------|-------|------|
| Edges form a cycle | `dag.ErrCycleDetected` | 422 |
| Unknown ref in edge (e.g. `from_node_ref: "xyz"` but no node has `ref: "xyz"`) | `"dag: unknown from_node_ref \"xyz\""` | 500 |
| Empty `dag.ID` | DB constraint error | 500 |
| Duplicate node IDs | DB primary key violation | 500 |
| DB connection lost | Wrapped pgx error | 500 |

#### Go usage

```go
result, err := store.CreateDAG(ctx, &dag.DAG{
    ID: "onboarding-form",
    Nodes: []dag.Node{
        {Ref: "q1", Data: json.RawMessage(`{"question": "Your role?"}`)},
        {Ref: "q2", Data: json.RawMessage(`{"question": "Language?"}`)},
    },
    Edges: []dag.Edge{
        {FromNodeRef: "q1", ToNodeRef: "q2", Data: json.RawMessage(`{"answer": "Dev"}`)},
    },
})
if errors.Is(err, dag.ErrCycleDetected) {
    // handle cycle
}
// result.Nodes[0].ID → "d959db72-..."
// result.Nodes[1].ID → "bf82148f-..."
// result.Edges[0].FromNodeID → "d959db72-..."
```

#### curl

```bash
curl -X POST http://localhost:3000/dag \
  -H "Content-Type: application/json" \
  -d '{
    "id": "onboarding-form",
    "nodes": [
      { "ref": "q1", "data": { "question": "What is your role?" } },
      { "ref": "q2", "data": { "question": "Preferred language?" } }
    ],
    "edges": [
      { "from_node_ref": "q1", "to_node_ref": "q2", "data": { "answer": "Developer" } }
    ]
  }'
```

---

### GetDAG

```
GetDAG(ctx context.Context, dagID string) (*DAG, error)
```

Retrieves all nodes and edges for a DAG. Nodes and edges ordered by `created_at`.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Found | `*DAG` with nodes + edges | 200 |
| No nodes exist for dagID | `nil, nil` | 404 |
| DB error | `nil, error` | 500 |

**Output (200):**
```json
{
  "id": "onboarding-form",
  "nodes": [
    { "id": "d959db72-...", "data": { "question": "What is your role?" } },
    { "id": "bf82148f-...", "data": { "question": "Preferred language?" } }
  ],
  "edges": [
    { "id": "de054cf9-...", "from_node_id": "d959db72-...", "to_node_id": "bf82148f-...", "data": { "answer": "Developer" } }
  ]
}
```

**Output (404):**
```json
{ "error": "dag not found" }
```

#### Go usage

```go
d, err := store.GetDAG(ctx, "onboarding-form")
if err != nil {
    // DB error
}
if d == nil {
    // DAG does not exist
}
// use d.Nodes, d.Edges
```

#### curl

```bash
curl http://localhost:3000/dag/onboarding-form
```

---

### DeleteDAG

```
DeleteDAG(ctx context.Context, dagID string) error
```

Deletes all nodes and edges for a DAG in one transaction. **No error if dagID doesn't exist.**

| Scenario | Returns | HTTP |
|----------|---------|------|
| Deleted | `nil` | 204 |
| DAG didn't exist | `nil` | 204 |
| DB error | `error` | 500 |

#### Go usage

```go
err := store.DeleteDAG(ctx, "onboarding-form")
```

#### curl

```bash
curl -X DELETE http://localhost:3000/dag/onboarding-form
```

---

## Node Operations (Granular)

### AddNode

```
AddNode(ctx context.Context, dagID string, node *Node) (string, error)
```

Inserts a single node into a DAG. If `node.ID` is empty, a UUID is auto-generated. Returns the node ID.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Success (auto ID) | `("a1b2c3-...", nil)` | 201 |
| Success (provided ID) | `("custom-id", nil)` | 201 |
| Duplicate ID | `("", error)` — DB primary key violation | 500 |
| DB error | `("", error)` | 500 |

**Input (no ID — auto-generate):**
```json
{ "data": { "question": "What is your email?", "type": "text" } }
```

**Output (201):**
```json
{ "id": "946aac51-397c-4b03-b702-977f93c40ab8" }
```

**Input (with ID):**
```json
{ "id": "q5", "data": { "question": "What is your email?", "type": "text" } }
```

**Output (201):**
```json
{ "id": "q5" }
```

#### Go usage

```go
// Auto-generate ID
id, err := store.AddNode(ctx, "form-1", &dag.Node{
    Data: json.RawMessage(`{"question": "Email?"}`),
})
// id → "946aac51-..."

// Provide ID
id, err := store.AddNode(ctx, "form-1", &dag.Node{
    ID:   "q5",
    Data: json.RawMessage(`{"question": "Email?"}`),
})
// id → "q5"
```

#### curl

```bash
# Auto-generate ID
curl -X POST http://localhost:3000/dag/form-1/nodes \
  -H "Content-Type: application/json" \
  -d '{"data":{"question":"Email?","type":"text"}}'

# Provide ID
curl -X POST http://localhost:3000/dag/form-1/nodes \
  -H "Content-Type: application/json" \
  -d '{"id":"q5","data":{"question":"Email?","type":"text"}}'
```

---

### GetNode

```
GetNode(ctx context.Context, nodeID string) (*Node, error)
```

Fetches a single node by its primary key.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Found | `*Node` | 200 |
| Not found | `nil, nil` | 404 |
| DB error | `nil, error` | 500 |

**Output (200):**
```json
{ "id": "q5", "data": { "question": "Email?", "type": "text" } }
```

**Output (404):**
```json
{ "error": "node not found" }
```

#### Go usage

```go
n, err := store.GetNode(ctx, "q5")
if err != nil {
    // DB error
}
if n == nil {
    // node does not exist
}
```

#### curl

```bash
curl http://localhost:3000/nodes/q5
```

---

### UpdateNode

```
UpdateNode(ctx context.Context, node *Node) error
```

Updates the `data` field of an existing node. `node.ID` is **required**. Only `data` is updated — `dag_id` and `created_at` remain unchanged.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Updated | `nil` | 204 |
| Node doesn't exist | `dag.ErrNodeNotFound` | 404 |
| DB error | `error` | 500 |

**Input:**
```json
{ "data": { "question": "Updated question text?", "type": "textarea" } }
```

Note: `id` comes from the URL path parameter, not the body.

**Output (204):** No body.

**Output (404):**
```json
{ "error": "node not found" }
```

#### Go usage

```go
err := store.UpdateNode(ctx, &dag.Node{
    ID:   "q5",
    Data: json.RawMessage(`{"question": "Updated?", "type": "textarea"}`),
})
if errors.Is(err, dag.ErrNodeNotFound) {
    // node doesn't exist
}
```

#### curl

```bash
curl -X PUT http://localhost:3000/nodes/q5 \
  -H "Content-Type: application/json" \
  -d '{"data":{"question":"Updated question?","type":"textarea"}}'
```

---

### DeleteNode

```
DeleteNode(ctx context.Context, nodeID string) error
```

Deletes a node by its ID. **Cascade-deletes all edges** referencing this node (both `from_node_id` and `to_node_id`). **No error if not found.**

| Scenario | Returns | HTTP |
|----------|---------|------|
| Deleted (+ cascade edges) | `nil` | 204 |
| Node didn't exist | `nil` | 204 |
| DB error | `error` | 500 |

**Output (204):** No body.

#### Go usage

```go
err := store.DeleteNode(ctx, "q5")
// All edges referencing q5 are automatically deleted
```

#### curl

```bash
curl -X DELETE http://localhost:3000/nodes/q5
```

---

### ListNodes

```
ListNodes(ctx context.Context, dagID string) ([]Node, error)
```

Returns all nodes for a DAG, ordered by `created_at`. Returns **empty slice `[]`** (not nil) if none found.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Nodes found | `[]Node{...}` | 200 |
| No nodes | `[]Node{}` (empty slice) | 200 |
| DB error | `nil, error` | 500 |

**Output (200, with nodes):**
```json
[
  { "id": "d959db72-...", "data": { "question": "What is your role?", "type": "select" } },
  { "id": "bf82148f-...", "data": { "question": "Preferred language?", "type": "select" } }
]
```

**Output (200, empty):**
```json
[]
```

#### Go usage

```go
nodes, err := store.ListNodes(ctx, "form-1")
// len(nodes) == 0 means no nodes, but never nil
```

#### curl

```bash
curl http://localhost:3000/dag/form-1/nodes
```

---

## Edge Operations (Granular)

### AddEdge

```
AddEdge(ctx context.Context, dagID string, edge *Edge) (string, error)
```

Inserts a single edge. If `edge.ID` is empty, a UUID is auto-generated. **Validates acyclic** — fetches all existing edges for the DAG, appends the new one, runs cycle detection. Only inserts if no cycle.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Success (auto ID) | `("a1b2c3-...", nil)` | 201 |
| Success (provided ID) | `("e5", nil)` | 201 |
| Would create cycle | `("", dag.ErrCycleDetected)` | 422 |
| Referenced node doesn't exist | `("", error)` — DB FK violation | 500 |
| Duplicate edge ID | `("", error)` — DB PK violation | 500 |
| DB error | `("", error)` | 500 |

**Input (no ID):**
```json
{ "from_node_id": "d959db72-...", "to_node_id": "bf82148f-...", "data": { "answer": "Developer" } }
```

**Output (201):**
```json
{ "id": "0249ec8e-433c-4fc4-9ffc-9f4821f945e7" }
```

**Input (with ID):**
```json
{ "id": "e5", "from_node_id": "d959db72-...", "to_node_id": "bf82148f-...", "data": { "answer": "Developer" } }
```

**Output (201):**
```json
{ "id": "e5" }
```

**Output (422, cycle detected):**
```json
{ "error": "cycle detected" }
```

#### Cycle example

Given: `q1 → q2 → q3`

Adding `q3 → q1` would create a cycle → returns `ErrCycleDetected`.

#### Go usage

```go
id, err := store.AddEdge(ctx, "form-1", &dag.Edge{
    FromNodeID: "q1",
    ToNodeID:   "q2",
    Data:       json.RawMessage(`{"answer": "Developer"}`),
})
if errors.Is(err, dag.ErrCycleDetected) {
    // edge would create a cycle
}
```

#### curl

```bash
curl -X POST http://localhost:3000/dag/form-1/edges \
  -H "Content-Type: application/json" \
  -d '{"from_node_id":"d959db72-...","to_node_id":"bf82148f-...","data":{"answer":"Developer"}}'
```

---

### GetEdge

```
GetEdge(ctx context.Context, edgeID string) (*Edge, error)
```

Fetches a single edge by its primary key.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Found | `*Edge` | 200 |
| Not found | `nil, nil` | 404 |
| DB error | `nil, error` | 500 |

**Output (200):**
```json
{ "id": "de054cf9-...", "from_node_id": "d959db72-...", "to_node_id": "bf82148f-...", "data": { "answer": "Developer" } }
```

**Output (404):**
```json
{ "error": "edge not found" }
```

#### Go usage

```go
e, err := store.GetEdge(ctx, "de054cf9-...")
if err != nil {
    // DB error
}
if e == nil {
    // edge does not exist
}
```

#### curl

```bash
curl http://localhost:3000/edges/de054cf9-...
```

---

### UpdateEdge

```
UpdateEdge(ctx context.Context, edge *Edge) error
```

Updates `from_node_id`, `to_node_id`, and `data` of an existing edge. `edge.ID` is **required**. **Validates acyclic** — loads all existing edges, replaces the updated one, runs cycle detection.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Updated | `nil` | 204 |
| Edge doesn't exist | `dag.ErrEdgeNotFound` | 404 |
| Update would create cycle | `dag.ErrCycleDetected` | 422 |
| Referenced node doesn't exist | `error` — DB FK violation | 500 |
| DB error | `error` | 500 |

**Input:**
```json
{ "from_node_id": "q1", "to_node_id": "q3", "data": { "answer": "changed" } }
```

Note: `id` comes from the URL path parameter, not the body.

**Output (204):** No body.

**Output (404):**
```json
{ "error": "edge not found" }
```

**Output (422):**
```json
{ "error": "cycle detected" }
```

#### Go usage

```go
err := store.UpdateEdge(ctx, &dag.Edge{
    ID:         "e1",
    FromNodeID: "q1",
    ToNodeID:   "q3",
    Data:       json.RawMessage(`{"answer": "changed"}`),
})
if errors.Is(err, dag.ErrEdgeNotFound) {
    // edge doesn't exist
}
if errors.Is(err, dag.ErrCycleDetected) {
    // update would create a cycle
}
```

#### curl

```bash
curl -X PUT http://localhost:3000/edges/e1 \
  -H "Content-Type: application/json" \
  -d '{"from_node_id":"q1","to_node_id":"q3","data":{"answer":"changed"}}'
```

---

### DeleteEdge

```
DeleteEdge(ctx context.Context, edgeID string) error
```

Deletes an edge by its ID. **No error if not found.**

| Scenario | Returns | HTTP |
|----------|---------|------|
| Deleted | `nil` | 204 |
| Edge didn't exist | `nil` | 204 |
| DB error | `error` | 500 |

**Output (204):** No body.

#### Go usage

```go
err := store.DeleteEdge(ctx, "e1")
```

#### curl

```bash
curl -X DELETE http://localhost:3000/edges/e1
```

---

### ListEdges

```
ListEdges(ctx context.Context, dagID string) ([]Edge, error)
```

Returns all edges for a DAG, ordered by `created_at`. Returns **empty slice `[]`** (not nil) if none found.

| Scenario | Returns | HTTP |
|----------|---------|------|
| Edges found | `[]Edge{...}` | 200 |
| No edges | `[]Edge{}` (empty slice) | 200 |
| DB error | `nil, error` | 500 |

**Output (200, with edges):**
```json
[
  { "id": "de054cf9-...", "from_node_id": "d959db72-...", "to_node_id": "bf82148f-...", "data": { "answer": "Developer" } },
  { "id": "a1c4e839-...", "from_node_id": "d959db72-...", "to_node_id": "ab54f559-...", "data": { "answer": "Designer" } }
]
```

**Output (200, empty):**
```json
[]
```

#### Go usage

```go
edges, err := store.ListEdges(ctx, "form-1")
// len(edges) == 0 means no edges, but never nil
```

#### curl

```bash
curl http://localhost:3000/dag/form-1/edges
```

---

## ID Generation Rules

| Operation | `id` field empty | `id` field provided |
|-----------|-----------------|---------------------|
| **CreateDAG** nodes | UUID auto-generated | Used as-is |
| **CreateDAG** edges | UUID auto-generated | Used as-is |
| **AddNode** | UUID auto-generated, returned | Used as-is, returned |
| **AddEdge** | UUID auto-generated, returned | Used as-is, returned |
| **UpdateNode** | **Error** — ID is required | Updates that node |
| **UpdateEdge** | **Error** — ID is required | Updates that edge |

**UUID format:** v4, generated by `github.com/google/uuid` (app-side, not DB-side).

**Example:** `d959db72-bf20-4d96-9b39-4c2c5371c355`

---

## Cycle Detection

Cycle detection uses **DFS (Depth-First Search)** topological analysis.

### When it runs

| Method | Cycle check |
|--------|-------------|
| `CreateDAG` | Yes — validates all edges before inserting |
| `AddEdge` | Yes — loads existing edges, appends new one, validates |
| `UpdateEdge` | Yes — loads existing edges, replaces updated one, validates |
| `GetDAG` | No |
| `DeleteDAG` | No |
| All node operations | No |
| `GetEdge` | No |
| `DeleteEdge` | No |
| `ListEdges` | No |

### How it works

1. Build adjacency list from all edges
2. Run DFS from every unvisited node
3. If a node is visited while still "in progress" → **cycle detected**
4. Returns `dag.ErrCycleDetected` before any DB write

### Example

```
Existing: q1 → q2 → q3

AddEdge(q3 → q1)  →  ErrCycleDetected  (q1 → q2 → q3 → q1 is a cycle)
AddEdge(q3 → q4)  →  OK                (no cycle)
AddEdge(q2 → q3)  →  OK                (duplicate edge, not a cycle)
```

---

## Error Handling Guide

### All possible error types

| Error | Type | When | How to check |
|-------|------|------|-------------|
| Cycle detected | Sentinel | `CreateDAG`, `AddEdge`, `UpdateEdge` | `errors.Is(err, dag.ErrCycleDetected)` |
| Node not found | Sentinel | `UpdateNode` | `errors.Is(err, dag.ErrNodeNotFound)` |
| Edge not found | Sentinel | `UpdateEdge` | `errors.Is(err, dag.ErrEdgeNotFound)` |
| Unknown ref | Runtime | `CreateDAG` with bad ref | `strings.Contains(err.Error(), "unknown from_node_ref")` or `"unknown to_node_ref"` |
| Primary key violation | DB | `AddNode`/`AddEdge` with duplicate ID | Wrapped pgx error |
| Foreign key violation | DB | `AddEdge`/`UpdateEdge` referencing non-existent node | Wrapped pgx error |
| Connection error | DB | Any method | Wrapped pgx error |
| Transaction error | DB | `CreateDAG`, `DeleteDAG` | Wrapped pgx error, prefixed `"dag: begin tx:"` or `"dag: commit:"` |

### Error prefix convention

All errors from the postgres implementation are prefixed with `"dag: "`:

```
"dag: cycle detected, graph is not acyclic"
"dag: node not found"
"dag: edge not found"
"dag: unknown from_node_ref \"xyz\""
"dag: insert node abc: ..."
"dag: begin tx: ..."
"dag: commit: ..."
"dag: query nodes: ..."
```

### Complete error handling pattern (Fiber)

```go
func handleError(c fiber.Ctx, err error) error {
    if err == nil {
        return nil
    }

    // Sentinel errors → known HTTP statuses
    if errors.Is(err, dag.ErrCycleDetected) {
        return c.Status(422).JSON(fiber.Map{"error": "cycle detected"})
    }
    if errors.Is(err, dag.ErrNodeNotFound) {
        return c.Status(404).JSON(fiber.Map{"error": "node not found"})
    }
    if errors.Is(err, dag.ErrEdgeNotFound) {
        return c.Status(404).JSON(fiber.Map{"error": "edge not found"})
    }

    // Everything else → 500
    return c.Status(500).JSON(fiber.Map{"error": err.Error()})
}
```

---

## HTTP Status Code Mapping

| Status | When |
|--------|------|
| **200** | Successful read (GET) or schema operation |
| **201** | Successful create (POST /dag, POST /nodes, POST /edges) |
| **204** | Successful update (PUT) or delete (DELETE) — no body |
| **400** | Invalid JSON body / malformed request |
| **404** | Resource not found (GetDAG nil, GetNode nil, GetEdge nil, UpdateNode/UpdateEdge on missing ID) |
| **422** | Cycle detected (CreateDAG, AddEdge, UpdateEdge) |
| **500** | DB error, unknown ref, FK violation, PK violation, connection error |

### Endpoint → Method → Status matrix

| Endpoint | Method | Success | Not Found | Cycle | Error |
|----------|--------|---------|-----------|-------|-------|
| `POST /schema` | CreateSchema | 200 | — | — | 500 |
| `DELETE /schema` | DropSchema | 200 | — | — | 500 |
| `POST /dag` | CreateDAG | 201 | — | 422 | 500 |
| `GET /dag/:id` | GetDAG | 200 | 404 | — | 500 |
| `DELETE /dag/:id` | DeleteDAG | 204 | 204 | — | 500 |
| `POST /dag/:id/nodes` | AddNode | 201 | — | — | 500 |
| `GET /dag/:id/nodes` | ListNodes | 200 | 200 `[]` | — | 500 |
| `GET /nodes/:id` | GetNode | 200 | 404 | — | 500 |
| `PUT /nodes/:id` | UpdateNode | 204 | 404 | — | 500 |
| `DELETE /nodes/:id` | DeleteNode | 204 | 204 | — | 500 |
| `POST /dag/:id/edges` | AddEdge | 201 | — | 422 | 500 |
| `GET /dag/:id/edges` | ListEdges | 200 | 200 `[]` | — | 500 |
| `GET /edges/:id` | GetEdge | 200 | 404 | — | 500 |
| `PUT /edges/:id` | UpdateEdge | 204 | 404 | 422 | 500 |
| `DELETE /edges/:id` | DeleteEdge | 204 | 204 | — | 500 |

---

## Migration & Schema Management

### First-time setup

Call `CreateSchema` on app startup. It uses `IF NOT EXISTS` — safe to run every time.

```go
store.CreateSchema(ctx)
```

Or run the SQL manually:

```bash
psql $DATABASE_URL -f schema.sql
```

### Drop everything (destructive)

```go
store.DropSchema(ctx)
```

Or via HTTP:

```bash
curl -X DELETE http://localhost:3000/schema
```

Or SQL:

```bash
psql $DATABASE_URL -c "DROP TABLE IF EXISTS dag_edges, dag_nodes CASCADE;"
```

### Reset (drop + recreate)

```go
store.DropSchema(ctx)
store.CreateSchema(ctx)
```

Or via curl:

```bash
curl -X DELETE http://localhost:3000/schema
curl -X POST http://localhost:3000/schema
```

### Adding columns (future migrations)

The schema uses `IF NOT EXISTS` for idempotent creation. For schema changes (adding columns, etc.), use standard SQL migrations:

```sql
-- Example: add a "label" column to nodes
ALTER TABLE dag_nodes ADD COLUMN IF NOT EXISTS label TEXT DEFAULT '';

-- Example: add an "updated_at" column
ALTER TABLE dag_nodes ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
ALTER TABLE dag_edges ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
```

Run these via `psql`, a migration tool (goose, migrate, atlas), or add them to `CreateSchema`.

### Checking current schema

```bash
psql $DATABASE_URL -c "\d dag_nodes"
psql $DATABASE_URL -c "\d dag_edges"
```

---

## Fiber Integration (Full Example)

The `server/main.go` provides a complete Fiber v3 HTTP server with all 16 endpoints. Run it:

```bash
export DATABASE_URL='postgresql://user:pass@host/db?sslmode=require'
go run ./server/
```

### All endpoints

```
POST   /schema              → CreateSchema
DELETE /schema              → DropSchema

POST   /dag                 → CreateDAG
GET    /dag/:id             → GetDAG
DELETE /dag/:id             → DeleteDAG

POST   /dag/:id/nodes       → AddNode
GET    /dag/:id/nodes       → ListNodes
GET    /nodes/:id           → GetNode
PUT    /nodes/:id           → UpdateNode
DELETE /nodes/:id           → DeleteNode

POST   /dag/:id/edges       → AddEdge
GET    /dag/:id/edges       → ListEdges
GET    /edges/:id           → GetEdge
PUT    /edges/:id           → UpdateEdge
DELETE /edges/:id           → DeleteEdge
```

### End-to-end curl test script

```bash
BASE=http://localhost:3000

# Setup
curl -X POST $BASE/schema

# Create DAG with refs
curl -X POST $BASE/dag -H "Content-Type: application/json" \
  -d '{"id":"test","nodes":[{"ref":"a","data":{"q":"Q1"}},{"ref":"b","data":{"q":"Q2"}}],"edges":[{"from_node_ref":"a","to_node_ref":"b","data":{"answer":"next"}}]}'

# Read it back
curl $BASE/dag/test

# Add a node
curl -X POST $BASE/dag/test/nodes -H "Content-Type: application/json" \
  -d '{"data":{"q":"Q3"}}'

# List nodes
curl $BASE/dag/test/nodes

# List edges
curl $BASE/dag/test/edges

# Cleanup
curl -X DELETE $BASE/dag/test
curl -X DELETE $BASE/schema
```
