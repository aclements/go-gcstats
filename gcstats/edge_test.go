// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

import (
	"reflect"
	"testing"
)

func checkEdges(t *testing.T, test []uniform, expect []edge) {
	got := uniformSumToEdges(test)
	if !reflect.DeepEqual(expect, got) {
		t.Errorf("failed to find edges for\n  %v\nexpected %v\ngot      %v", test, expect, got)
	}
}

func TestEdgesNonOverlapping(t *testing.T) {
	test := []uniform{{0, 1, 0.5}, {2, 3, 0.5}}
	expect := []edge{{0, 0.5, 0}, {1, 0, 0}, {2, 0.5, 0}, {3, 0, 0}}
	checkEdges(t, test, expect)
}

func TestEdgesOverlapping(t *testing.T) {
	test := []uniform{{0, 1, 0.5}, {0.5, 1.5, 0.5}}
	expect := []edge{{0, 0.5, 0}, {0.5, 1, 0}, {1, 0.5, 0}, {1.5, 0, 0}}
	checkEdges(t, test, expect)
}

func TestEdgesNested(t *testing.T) {
	test := []uniform{{0, 2, 0.5}, {0.5, 1.5, 0.5}}
	expect := []edge{{0, 0.25, 0}, {0.5, 0.75, 0}, {1.5, 0.25, 0}, {2, 0, 0}}
	checkEdges(t, test, expect)
}

func TestEdgesAligned(t *testing.T) {
	test := []uniform{{0, 2, 0.5}, {0, 1, 0.25}, {1, 2, 0.25}}
	expect := []edge{{0, 0.5, 0}, {2, 0, 0}}
	checkEdges(t, test, expect)
}

func TestEdgesDeltas(t *testing.T) {
	test := []uniform{{0, 0, 0.5}, {1, 1, 0.5}}
	expect := []edge{{0, 0, 0.5}, {1, 0, 0.5}}
	checkEdges(t, test, expect)
}

func TestEdgesNestedDelta(t *testing.T) {
	test := []uniform{{0, 1, 0.5}, {0.5, 0.5, 0.5}}
	expect := []edge{{0, 0.5, 0}, {0.5, 0.5, 0.5}, {1, 0, 0}}
	checkEdges(t, test, expect)
}

func TestEdgesAlignedDeltas(t *testing.T) {
	test := []uniform{{0, 1, 0.25}, {1, 1, 0.25}, {1, 1, 0.25}, {1, 2, 0.25}}
	expect := []edge{{0, 0.25, 0}, {1, 0.25, 0.5}, {2, 0, 0}}
	checkEdges(t, test, expect)
}
