// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

import "testing"

//           ━━━━━━━━━━━━━━━━━━━━           1
//           ▏                  ▕           0.75
//           ▏                  ▕           0.5   util
//           ▏                  ▕           0.25
// ━━━━━━━━━━--------------------━━━━━━━━━━ 0
// 0        25        50        75       100 time
var statsQuarters = GcStats{[]Phase{
	{Begin: 0, Duration: 25, Gomaxprocs: 4, GCProcs: 4},
	{Begin: 25, Duration: 50, Gomaxprocs: 4, GCProcs: 0},
	{Begin: 75, Duration: 25, Gomaxprocs: 4, GCProcs: 4},
}, 1}

func testMUDCDF(t *testing.T, mud *MUD, x, cdf float64) {
	got := mud.CDF(x)
	if cdf != got {
		t.Errorf("expected CDF(%v)=%v, got %v", x, cdf, got)
	}
}

func testMUDInvCDF(t *testing.T, mud *MUD, cdf, x float64) {
	got := mud.InvCDF(cdf)
	if x != got {
		t.Errorf("expected InvCDF(%v)=%v, got %v", cdf, x, got)
	}
}

func testMUD(t *testing.T, mud *MUD, x, cdf float64) {
	testMUDCDF(t, mud, x, cdf)
	testMUDInvCDF(t, mud, cdf, x)
}

func TestQuartersMUD0(t *testing.T) {
	// ↑∫=0.5             ↑∫=0.5
	// │                  │      PDF
	// ╵------------------╵ 0.0
	// 0       util       1
	mud := statsQuarters.MutatorUtilizationDistribution(0)
	testMUDCDF(t, mud, 0, 0.5)
	testMUDInvCDF(t, mud, 0, 0)
	testMUDInvCDF(t, mud, 0.25, 0)
	testMUDInvCDF(t, mud, 0.5, 1) // 0 or 1 are acceptable
	testMUDInvCDF(t, mud, 0.75, 1)
	testMUDInvCDF(t, mud, 1, 1)
	testMUDCDF(t, mud, 0.5, 0.5)
	testMUD(t, mud, 1, 1)
}

func TestQuartersMUD25(t *testing.T) {
	//                    ↑∫=1/3
	//                    │
	// ┍━━━━━━━━━━━━━━━━━━┥ 2/3  PDF
	// │                  │ 1/3
	// ╵------------------╵ 0/3
	// 0       util       1
	mud := statsQuarters.MutatorUtilizationDistribution(25)
	testMUD(t, mud, 0, 0)
	testMUD(t, mud, 0.25, 1/6.0)
	testMUD(t, mud, 0.5, 1/3.0)
	testMUD(t, mud, 0.75, 3/6.0)
	testMUD(t, mud, 1, 1)
}

func TestQuartersMUD50(t *testing.T) {
	//           ┍━━━━━━━━┑ 2.0
	//           │        │ 1.5
	//           │        │ 1.0  PDF
	//           │        │ 0.5
	// ━━━━━━━━━━┙--------╵ 0.0
	// 0       util       1
	mud := statsQuarters.MutatorUtilizationDistribution(50)
	testMUD(t, mud, 0.5, 0)
	testMUD(t, mud, 0.75, 0.5)
	testMUD(t, mud, 1, 1)
}

func TestQuartersMUD100(t *testing.T) {
	//           ↑∫=1
	//           │               PDF
	// ----------╵--------- 0.0
	// 0       util       1
	mud := statsQuarters.MutatorUtilizationDistribution(100)
	testMUDCDF(t, mud, 0.499, 0)
	testMUD(t, mud, 0.5, 1)
	testMUDCDF(t, mud, 0.501, 1)
	for _, y := range []float64{0, 0.25, 0.5, 0.75, 1} {
		inv := mud.InvCDF(y)
		if inv != 0.5 {
			t.Errorf("expected InvCDF(%v)=%v, got %v", y, 0.5, inv)
		}
	}
}

// TODO: Test delta in the middle of a non-zero region.
