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
