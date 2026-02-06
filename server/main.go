package main

import (
	"context"
	"errors"
	"log"
	"os"

	"github.com/meikuraledutech/dag"
	"github.com/meikuraledutech/dag/postgres"
	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	var store dag.Store = postgres.New(pool)

	app := fiber.New()

	// ── Schema ────────────────────────────────────────────────────────
	app.Post("/schema", func(c fiber.Ctx) error {
		if err := store.CreateSchema(c.Context()); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "schema created"})
	})

	app.Delete("/schema", func(c fiber.Ctx) error {
		if err := store.DropSchema(c.Context()); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"message": "schema dropped"})
	})

	// ── DAG (bulk) ────────────────────────────────────────────────────
	app.Post("/dag", func(c fiber.Ctx) error {
		var d dag.DAG
		if err := c.Bind().JSON(&d); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
		}
		result, err := store.CreateDAG(c.Context(), &d)
		if errors.Is(err, dag.ErrCycleDetected) {
			return c.Status(422).JSON(fiber.Map{"error": "cycle detected"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(result)
	})

	app.Get("/dag/:id", func(c fiber.Ctx) error {
		d, err := store.GetDAG(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if d == nil {
			return c.Status(404).JSON(fiber.Map{"error": "dag not found"})
		}
		return c.JSON(d)
	})

	app.Delete("/dag/:id", func(c fiber.Ctx) error {
		if err := store.DeleteDAG(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	// ── Nodes ─────────────────────────────────────────────────────────
	app.Post("/dag/:id/nodes", func(c fiber.Ctx) error {
		var node dag.Node
		if err := c.Bind().JSON(&node); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
		}
		id, err := store.AddNode(c.Context(), c.Params("id"), &node)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"id": id})
	})

	app.Get("/dag/:id/nodes", func(c fiber.Ctx) error {
		nodes, err := store.ListNodes(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(nodes)
	})

	app.Get("/nodes/:id", func(c fiber.Ctx) error {
		n, err := store.GetNode(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if n == nil {
			return c.Status(404).JSON(fiber.Map{"error": "node not found"})
		}
		return c.JSON(n)
	})

	app.Put("/nodes/:id", func(c fiber.Ctx) error {
		var node dag.Node
		if err := c.Bind().JSON(&node); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
		}
		node.ID = c.Params("id")
		err := store.UpdateNode(c.Context(), &node)
		if errors.Is(err, dag.ErrNodeNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "node not found"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	app.Delete("/nodes/:id", func(c fiber.Ctx) error {
		if err := store.DeleteNode(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	// ── Edges ─────────────────────────────────────────────────────────
	app.Post("/dag/:id/edges", func(c fiber.Ctx) error {
		var edge dag.Edge
		if err := c.Bind().JSON(&edge); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
		}
		id, err := store.AddEdge(c.Context(), c.Params("id"), &edge)
		if errors.Is(err, dag.ErrCycleDetected) {
			return c.Status(422).JSON(fiber.Map{"error": "cycle detected"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(fiber.Map{"id": id})
	})

	app.Get("/dag/:id/edges", func(c fiber.Ctx) error {
		edges, err := store.ListEdges(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(edges)
	})

	app.Get("/edges/:id", func(c fiber.Ctx) error {
		e, err := store.GetEdge(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if e == nil {
			return c.Status(404).JSON(fiber.Map{"error": "edge not found"})
		}
		return c.JSON(e)
	})

	app.Put("/edges/:id", func(c fiber.Ctx) error {
		var edge dag.Edge
		if err := c.Bind().JSON(&edge); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
		}
		edge.ID = c.Params("id")
		err := store.UpdateEdge(c.Context(), &edge)
		if errors.Is(err, dag.ErrEdgeNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "edge not found"})
		}
		if errors.Is(err, dag.ErrCycleDetected) {
			return c.Status(422).JSON(fiber.Map{"error": "cycle detected"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	app.Delete("/edges/:id", func(c fiber.Ctx) error {
		if err := store.DeleteEdge(c.Context(), c.Params("id")); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(204)
	})

	log.Fatal(app.Listen(":3000"))
}
