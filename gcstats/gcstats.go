// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

// Phase represents the times for a single phase of a garbage
// collection cycle.
type Phase struct {
	// This phase spans nanoseconds [Begin, Begin+Duration).
	//
	// If absolute times are unknown, Begin is 0 and Duration may
	// be -1.
	Begin, Duration int64

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

// End returns the end time of p, or panics of p's duration is unknown.
func (p Phase) End() int64 {
	if p.Duration == -1 {
		panic("phase has unknown duration")
	}
	return p.Begin + p.Duration
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
	// Log of phases in order. These are assumed to span every
	// moment of program execution, though the exact duration of
	// phases spanning GC cycles may not be known.
	log []Phase
	n   int // # of GCs

	// progTimes indicates that phases have begin times that
	// indicate when they happened during program execution.
	//
	// If true, log[i].Begin+log[i].Duration == log[i+1].Begin.
	progTimes bool
}

// HaveProgTimes returns true if the log has begin times and hence
// indicates when phases happened during program execution.
//
// Without this information, one can still determine properties of
// phase durations, but not properties over program execution time.
func (s *GcStats) HaveProgTimes() bool {
	return s.progTimes
}

// Count returns the number of recorded garbage collections.
func (s *GcStats) Count() int {
	return s.n
}

// Phases returns a slice of recorded garbage collection phases.
func (s *GcStats) Phases() []Phase {
	return s.log
}

// Stops returns a slice of all stop-the-world phases. If multiple STW
// phases occur in succession, this joins them into a single phase and
// averages their CPU utilization. If the joined phases have multiple
// phase kinds, the joined phase will have kind PhaseMultiple.
func (s *GcStats) Stops() []Phase {
	stw := []Phase{}
	join := false
	for _, phase := range s.log {
		if !phase.STW {
			join = false
			continue
		}
		if join {
			// Join with previous STW
			prev := stw[len(stw)-1]
			dur1 := prev.Duration
			dur2 := phase.Duration
			f := float64(dur1) / float64(dur1+dur2)
			prev.GCProcs = prev.GCProcs*f + phase.GCProcs*(1-f)

			prev.Duration += dur2
			if prev.Kind != phase.Kind {
				prev.Kind = PhaseMultiple
			}
			stw[len(stw)-1] = prev
			continue
		}
		stw = append(stw, phase)
		join = true
	}
	return stw
}

// MaxPause returns the maximum pause time in nanoseconds.
func (s *GcStats) MaxPause() int64 {
	maxpause := int64(0)
	for _, phase := range s.Stops() {
		if phase.Duration > maxpause {
			maxpause = phase.Duration
		}
	}
	return maxpause
}
