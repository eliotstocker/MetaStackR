package dag

import (
	"errors"
	"fmt"
	"sort"
)

var ErrCycleDetected = errors.New("circular dependency detected in submodules")

type Graph struct {
	nodes        map[string]interface{}
	dependencies map[string][]string // node -> list of nodes it depends on
	dependents   map[string][]string // node -> list of nodes depending on it
}

func NewGraph() *Graph {
	return &Graph{
		nodes:        make(map[string]interface{}),
		dependencies: make(map[string][]string),
		dependents:   make(map[string][]string),
	}
}

func (g *Graph) AddNode(id string, data interface{}) {
	g.nodes[id] = data
	if _, exists := g.dependencies[id]; !exists {
		g.dependencies[id] = []string{}
	}
	if _, exists := g.dependents[id]; !exists {
		g.dependents[id] = []string{}
	}
}

// AddDependency specifies that 'nodeID' depends on 'dependsOnID'.
// Therefore, 'dependsOnID' MUST be merged/processed BEFORE 'nodeID'.
func (g *Graph) AddDependency(nodeID, dependsOnID string) {
	if _, exists := g.nodes[nodeID]; !exists {
		g.AddNode(nodeID, nil)
	}
	if _, exists := g.nodes[dependsOnID]; !exists {
		g.AddNode(dependsOnID, nil)
	}

	g.dependencies[nodeID] = append(g.dependencies[nodeID], dependsOnID)
	g.dependents[dependsOnID] = append(g.dependents[dependsOnID], nodeID)
}

// TopologicalSort returns nodes grouped into parallel execution batches (depth levels).
// Nodes in batch[0] have 0 dependencies and can be executed first in parallel.
// If a cycle is detected, returns ErrCycleDetected.
func (g *Graph) TopologicalSort() ([][]string, error) {
	if len(g.nodes) == 0 {
		return nil, nil
	}

	inDegree := make(map[string]int)
	for node := range g.nodes {
		inDegree[node] = len(g.dependencies[node])
	}

	var batches [][]string
	processedCount := 0

	for {
		var currentBatch []string
		for node, deg := range inDegree {
			if deg == 0 {
				currentBatch = append(currentBatch, node)
			}
		}

		if len(currentBatch) == 0 {
			break
		}

		// Sort batch deterministically for predictable execution order
		sort.Strings(currentBatch)
		batches = append(batches, currentBatch)

		// Mark current batch nodes as processed (-1) and decrement dependents' in-degrees
		for _, node := range currentBatch {
			inDegree[node] = -1
			processedCount++

			for _, dep := range g.dependents[node] {
				if inDegree[dep] > 0 {
					inDegree[dep]--
				}
			}
		}
	}

	if processedCount < len(g.nodes) {
		// Identify nodes involved in the cycle
		var cycleNodes []string
		for node, deg := range inDegree {
			if deg > 0 {
				cycleNodes = append(cycleNodes, node)
			}
		}
		sort.Strings(cycleNodes)
		return nil, fmt.Errorf("%w: cycle involves nodes %v", ErrCycleDetected, cycleNodes)
	}

	return batches, nil
}
