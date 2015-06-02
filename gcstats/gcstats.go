// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

// Phase represents the times for a single phase of a garbage
// collection cycle.
type Phase struct {
	// This phase spans nanoseconds [Begin, End).
	//
	// If absolute times are unknown, Begin is 0 and End is the
	// duration.
	//
	// TODO: Have Begin and Duration. Or possibly just Duration.
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
	// Log of phases. If progTimes, log[i].End == log[i+1].Begin for each i.
	log []Phase
	n   int // # of GCs

	// progTimes indicates that phases have begin times that
	// indicate when they happened during program execution.
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
			dur1 := prev.End - prev.Begin
			dur2 := phase.End - phase.Begin
			f := float64(dur1) / float64(dur1+dur2)
			prev.GCProcs = prev.GCProcs*f + phase.GCProcs*(1-f)

			if s.HaveProgTimes() {
				prev.End = phase.End
			} else {
				prev.End += dur2
			}
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
		if phase.End-phase.Begin > maxpause {
			maxpause = phase.End - phase.Begin
		}
	}
	return maxpause
}
