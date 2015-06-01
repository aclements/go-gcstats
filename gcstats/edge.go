// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

import "sort"

type edge struct {
	// At x, the function steps to value y until the next edge.
	x, y float64
	// Additionally at x is a Dirac delta function with dirac.
	dirac float64
}

type edges []edge

func (e edges) Len() int {
	return len(e)
}

func (e edges) Less(i, j int) bool {
	return e[i].x < e[j].x
}

func (e edges) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

// uniformSumToEdges converts a sum of scaled uniform distributions
// representing a step function to a sorted list of the edges of that
// step function.
func uniformSumToEdges(us []uniform) []edge {
	if len(us) == 0 {
		return []edge{{0, 0, 0}}
	}

	// Create initial edges. Here we use y as a height change,
	// rather than an absolute height.
	deltas := []edge{}
	for _, u := range us {
		if u.l == u.r {
			deltas = append(deltas, edge{u.l, 0, u.area})
		} else {
			h := u.area / (u.r - u.l)
			deltas = append(deltas, edge{u.l, h, 0}, edge{u.r, -h, 0})
		}
	}

	// Sort edges
	sort.Sort(edges(deltas))

	// Merge edges with identical x's and eliminate edges that
	// don't contribute anything
	out := 0
	for in, inedge := range deltas {
		if inedge.x != deltas[out].x {
			// Start a new edge. If the edge we were
			// building wound up with no contribution,
			// overwrite it.
			if deltas[out].y != 0 || deltas[out].dirac != 0 {
				out++
			}
			deltas[out] = inedge
		} else if in != out {
			// Merge in into es[out]
			deltas[out].y += inedge.y
			deltas[out].dirac += inedge.dirac
		}
	}
	deltas = deltas[:out+1]

	// Create output edge slice and compute absolute heights.
	edges := make([]edge, len(deltas))
	height := 0.0
	for i, delta := range deltas {
		height += delta.y
		edges[i] = edge{delta.x, height, delta.dirac}
	}

	return edges
}
