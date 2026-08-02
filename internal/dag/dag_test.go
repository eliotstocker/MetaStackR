package dag

import (
	"errors"
	"reflect"
	"testing"
)

func TestDAG_LinearDependencies(t *testing.T) {
	// A depends on B, B depends on C
	// Execution order must be: C -> B -> A
	g := NewGraph()
	g.AddDependency("A", "B")
	g.AddDependency("B", "C")

	batches, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := [][]string{
		{"C"},
		{"B"},
		{"A"},
	}

	if !reflect.DeepEqual(batches, expected) {
		t.Errorf("got batches %v, want %v", batches, expected)
	}
}

func TestDAG_ParallelBatches(t *testing.T) {
	// A depends on B and C
	// B and C have 0 dependencies
	// Execution order: [B, C] -> [A]
	g := NewGraph()
	g.AddDependency("A", "B")
	g.AddDependency("A", "C")

	batches, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := [][]string{
		{"B", "C"},
		{"A"},
	}

	if !reflect.DeepEqual(batches, expected) {
		t.Errorf("got batches %v, want %v", batches, expected)
	}
}

func TestDAG_DiamondDependencies(t *testing.T) {
	// A depends on B and C; B depends on D; C depends on D.
	// Execution order: [D] -> [B, C] -> [A]
	g := NewGraph()
	g.AddDependency("A", "B")
	g.AddDependency("A", "C")
	g.AddDependency("B", "D")
	g.AddDependency("C", "D")

	batches, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := [][]string{
		{"D"},
		{"B", "C"},
		{"A"},
	}

	if !reflect.DeepEqual(batches, expected) {
		t.Errorf("got batches %v, want %v", batches, expected)
	}
}

func TestDAG_CycleDetection(t *testing.T) {
	// A -> B -> C -> A
	g := NewGraph()
	g.AddDependency("A", "B")
	g.AddDependency("B", "C")
	g.AddDependency("C", "A")

	_, err := g.TopologicalSort()
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}

	if !errors.Is(err, ErrCycleDetected) {
		t.Errorf("expected ErrCycleDetected, got %v", err)
	}
}

func TestDAG_SingleNodeAndEmpty(t *testing.T) {
	g := NewGraph()
	batches, err := g.TopologicalSort()
	if err != nil || len(batches) != 0 {
		t.Errorf("expected empty batches for empty graph, got %v, err %v", batches, err)
	}

	g.AddNode("X", nil)
	batches, err = g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := [][]string{{"X"}}
	if !reflect.DeepEqual(batches, expected) {
		t.Errorf("got %v, want %v", batches, expected)
	}
}
