// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

// Phase represents the times for a single phase of a garbage
// collection cycle.
type Phase struct {
	// This phase spans nanoseconds [Begin, End).
	Begin, End int64

	// Kind of phase
	Kind PhaseKind

	// Garbage collection pass
	N int

	// GOMAXPROCS as of this phase
	Gomaxprocs int

	// Number of procs used by the garbage collector in this phase
	// (average over phase)
	GCProcs float64

	// Whether this phase was a STW phase
	STW bool
}

type PhaseKind int

//go:generate stringer -type=PhaseKind
const (
	PhaseSweepTerm PhaseKind = iota
	PhaseScan
	PhaseInstallWB
	PhaseMark
	PhaseMarkTerm
	PhaseSweep

	// PhaseMultiple represents multiple phases in one Phase.
	// This is only returned by aggregator functions.
	PhaseMultiple
)

type GcStats struct {
	// Log of phases. For each i, log[i].End == log[i+1].Begin.
	log []Phase
	n   int // # of GCs
}

// Count returns the number of recorded garbage collections.
func (s *GcStats) Count() int {
	return s.n
}

// Phases returns a slice of recorded garbage collection phases.
func (s *GcStats) Phases() []Phase {
	return s.log
}

// Stops returns a slice of all stop-the-world phases.
//
// The returned Phase slice may include PhaseMultiple phases if there
// are adjacent STW phases of different types in the log.
func (s *GcStats) Stops() []Phase {
	stw := []Phase{}
	for _, phase := range s.log {
		if !phase.STW {
			continue
		}
		if len(stw) > 0 {
			last := stw[len(stw)-1]
			if last.End == phase.Begin {
				// Join with previous STW
				f := float64(last.End-last.Begin) / float64(phase.End-last.Begin)
				last.GCProcs = last.GCProcs*f + phase.GCProcs*(1-f)

				last.End = phase.End
				if last.Kind != phase.Kind {
					last.Kind = PhaseMultiple
				}
				stw[len(stw)-1] = last
				continue
			}
		}
		stw = append(stw, phase)
	}
	return stw
}

// MaxPause returns the maximum pause time in nanoseconds.
func (s *GcStats) MaxPause() int64 {
	maxpause := int64(0)
	for _, phase := range s.Stops() {
		if phase.End-phase.Begin > maxpause {
			maxpause = phase.End - phase.Begin
		}
	}
	return maxpause
}
