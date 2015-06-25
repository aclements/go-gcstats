// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

import (
	"math"
	"sort"
)

// muInWindow returns the mutator utilization in the time window
// [begin, end). The utilization will be in the range [0, 1].
func muInWindow(begin, end int64, log []Phase) float64 {
	// If begin==end, compute instantaneous utilization.
	if begin == end {
		end++
	}

	// Compute the total time and GC time in [begin, end)
	totalNS := 0.0
	gcNS := 0.0
	for _, phase := range log {
		if phase.End() < begin {
			continue
		} else if phase.Begin >= end {
			break
		}

		// Section of this phase that overlaps the window
		pbegin := int64Max(begin, phase.Begin)
		pend := int64Min(end, phase.End())
		pdur := pend - pbegin

		gcprocs := phase.GCProcs
		if phase.STW {
			// GC may not use all of the procs, but the
			// mutator doesn't get any.
			gcprocs = float64(phase.Gomaxprocs)
		}
		gcNS += gcprocs * float64(pdur)
		totalNS += float64(int64(phase.Gomaxprocs) * pdur)
	}

	return (totalNS - gcNS) / totalNS
}

func (s *GcStats) requireProgTimes() {
	if !s.HaveProgTimes() {
		panic("computing mutator utilization requires program times in GC trace")
	}
}

// MutatorUtilization returns the mean mutator utilization between the
// first and last logged GC.
//
// This will panic if the trace does not have program execution times.
func (s *GcStats) MutatorUtilization() float64 {
	s.requireProgTimes()
	gcNS := float64(0)
	totalNS := int64(0)

	for _, phase := range s.log {
		gcNS += phase.GCProcs * float64(phase.Duration)
		totalNS += int64(phase.Gomaxprocs) * phase.Duration
	}
	return (float64(totalNS) - gcNS) / float64(totalNS)
}

// MMUs returns the minimum mutator utilization for each window size
// given in windowNS. Typically, MMU is plotted against a log scale
// of granularity.
//
// This will panic if the trace does not have program execution times.
func (s *GcStats) MMUs(windowNS []int) (mmu []float64) {
	// TODO: Add "sweep" as first phase in logged GC output so we
	// at least know the beginning of the program?

	mmu = make([]float64, len(windowNS))
	for i, window := range windowNS {
		mmu[i] = s.MMU(window)
	}
	return
}

// MMU returns a minimum mutator utilization at a granularity of
// windowNS nanoseconds. This is the minimum utilization for all
// windows of this size across the execution. The returned values are
// in the range [0, 1].
//
// This is equivalent to the 0th percentile of the mutator utilization
// distribution: s.MutatorUtilizationDistribution(windowNS).InvCDF(0),
// but is much faster to compute.
//
// This will panic if the trace does not have program execution times.
func (s *GcStats) MMU(windowNS int) (mmu float64) {
	s.requireProgTimes()
	if windowNS <= 0 {
		return 0
	}

	mmu = 1.0

	// We can think of the mutator utilization as a function of
	// the start time of the window. This function is continuous
	// and piecewise linear (unless windowNS==0, which we handled
	// above), where the boundaries between segments occur when
	// either edge of the window transitions from one phase to
	// another. Hence, the minimum of this function will always
	// occur when one of the edges of the window aligns with one
	// of the edges of a phase, so these are the only points we
	// need to consider.
	leftIdx := 0
	for i, phase := range s.log {
		// Consider the window starting at phase.Begin
		begin, end := phase.Begin, phase.Begin+int64(windowNS)
		if end <= s.log[len(s.log)-1].End() {
			// phase contains begin, so we can consider
			// the log starting at phase.
			util := muInWindow(begin, end, s.log[i:])
			mmu = math.Min(mmu, util)
		}

		// Consider the window ending at phase.End()
		begin, end = phase.End()-int64(windowNS), phase.End()
		if begin >= s.log[0].Begin {
			// This is a little trickier. We need to
			// consider the log starting at the phase
			// containing begin. Since it's monotonic, we
			// can search from where we were last.
			for s.log[leftIdx].End() < begin {
				leftIdx++
			}
			util := muInWindow(begin, end, s.log[leftIdx:])
			mmu = math.Min(mmu, util)
		}
	}
	return
}

// uniform is a uniform distribution over [l, r] scaled so the total
// weight is area. If l==r, this is a Dirac delta function.
type uniform struct {
	l, r, area float64
}

// MUD is a mutator utilization distribution for windows of size
// WindowNS.
//
// The domain of a MUD (the X axis) is mutator utilization and ranges
// from 0 (0% utilization) to 1 (100% utilization). The value of the
// distribution at x is the fraction of windows of size WindowNS over
// the entire execution that have mutator utilization x.
//
// MUDs are a generalization of minimum mutator utilization (MMU).
// The MMU is the 0th percentile (minimum) of the MUD. However, since
// the MMU is a minimum, it is not robust to outliers: a single slow
// garbage collection over an arbitrarily long execution has a
// significant effect on the MMU. On the other hand, higher
// percentiles of the MUD are robust to such outliers: the 1st
// percentile MUD for a 50ms window means "99% of the time, the
// program achieved at least this utilization over 50ms".
type MUD struct {
	WindowNS int
	edges    []edge
	csums    []float64
}

// MutatorUtilizationDistribution returns the mutator utilization
// distribution (MUD) for windows of size windowNS.
//
// This will panic if the trace does not have program execution times.
func (s *GcStats) MutatorUtilizationDistribution(windowNS int) *MUD {
	s.requireProgTimes()
	if len(s.log) == 0 {
		return &MUD{edges: []edge{{0, 0, 1}}, csums: []float64{0}}
	}

	// The distribution is the sum of many scaled uniform
	// distributions (some of which may have zero width). Compute
	// these.
	addends := []uniform{}

	// Compute first and last absolute time
	first, last := s.log[0].Begin, s.log[len(s.log)-1].End()

	// Cap the window at the duration of the log
	windowNS = int(int64Min(int64(windowNS), last-first))

	// [begin, end) is the current window. Slide it from
	// begin==first to begin==lastBegin.
	begin := first
	lastBegin := last - int64(windowNS)
	beginPhase, endPhase := 0, 0
	for begin < lastBegin {
		end := begin + int64(windowNS)

		// Find phases containing begin and end
		for s.log[beginPhase].End() <= begin {
			beginPhase++
		}
		for s.log[endPhase].End() <= end {
			endPhase++
		}

		// Create one uniform addend of the overall
		// distribution by sliding the window forward. We can
		// slide the window as long as both endpoints remain
		// in their same respective phase because the "height"
		// of the uniform addend will be constant for this.
		duration := int64Min(s.log[beginPhase].End()-begin, s.log[endPhase].End()-end)
		//fmt.Println(begin, end, duration, first, last, beginPhase, s.log[beginPhase], endPhase, s.log[endPhase])

		// Compute utilization at left edge of sliding window.
		// This is one edge of the uniform distribution.
		lutil := muInWindow(begin, end, s.log[beginPhase:])

		// Compute utilization at right edge of sliding
		// window. This is the other edge of the uniform
		// distribution. Note that the uniform distribution
		// *approaches* this utilization, but the support is
		// actually a half open interval (open at this end).
		// The below computation is still correct because
		// mutator utilization is a continuous function of
		// window position. We don't bother modeling this
		// because these infinitesimals don't matter for CDFs.
		rutil := muInWindow(begin+duration, end+duration, s.log[beginPhase:])

		// If the window size is 0, our continuity assumption
		// above is violated, but it's easy to fix: the
		// utilization will simply be constant for the
		// duration.
		if windowNS == 0 {
			rutil = lutil
		}

		// Ensure lutil <= rutil
		if lutil > rutil {
			lutil, rutil = rutil, lutil
		}

		// Finally, the area of this addend is the fraction we
		// just considered of the overall sliding window
		// interval.
		area := float64(duration) / float64(lastBegin-first)

		// Add it to the distribution
		addends = append(addends, uniform{lutil, rutil, area})

		begin += duration
	}

	// If lastBegin-first==0, the above logic has nowhere to slide
	// the window, so it doesn't produce any addends. Handle this
	// case here.
	if first == lastBegin {
		util := muInWindow(first, last, s.log)
		addends = append(addends, uniform{util, util, 1})
	}

	// Turn the collection of uniform addends into a sorted list
	// of edges of the resulting step function.
	edges := uniformSumToEdges(addends)

	// Compute cumulative sums. csums[i] is the sum up to, but
	// not including edges[i].
	csums := make([]float64, len(edges))
	for i, edge := range edges[:len(edges)-1] {
		w := edges[i+1].x - edge.x
		csums[i+1] = csums[i] + edge.y*w + edge.dirac
	}

	return &MUD{windowNS, edges, csums}
}

// CDF returns the fraction of windows for which the mutator
// utilization is <= util.
//
// This is the cumulative distribution function of the mutator
// utilization distribution.
func (d *MUD) CDF(util float64) (prob float64) {
	// Find the edge <= util
	righti := sort.Search(len(d.edges), func(n int) bool {
		return util < d.edges[n].x
	})
	if righti == 0 {
		return 0
	}
	lefti := righti - 1
	left := d.edges[lefti]

	// Compute the cumulative value
	return d.csums[lefti] + left.dirac + left.y*(util-left.x)
}

// InvCDF returns the pctile'th percentile mutator utilization: that
// is, the mutator utilization for which pctile percent of windows
// have mutator utilization <= util.
//
// InvCDF(0) returns the minimum mutator utilization (the 0th
// percentile utilization). InvCDF(1) returns the maximum mutator
// utilization. InvCDF(0.5) returns the median mutator utilization.
//
// This is the inverse cumulative distribution function of the mutator
// utilization distribution.
func (d *MUD) InvCDF(pctile float64) (util float64) {
	if pctile <= 0 {
		return d.edges[0].x
	} else if pctile >= 1 {
		return d.edges[len(d.edges)-1].x
	}

	// Find the cumulative sum <= pctile
	righti := sort.Search(len(d.csums), func(n int) bool {
		return pctile < d.csums[n]
	})
	if righti == 0 {
		// XXX I don't think this can happen
		return 0
	}
	lefti := righti - 1
	left := d.edges[lefti]

	// Compute the utilization
	if pctile < d.csums[lefti]+left.dirac {
		// pctile falls in the CDF discontinuity
		return left.x
	}
	return (pctile-d.csums[lefti]-left.dirac)/left.y + left.x
}
