// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Go 1.4 GODEBUG=gctrace=1 format, with optional start time
	gc14Log = regexp.MustCompile(`^gc(\d+)\(\d+\): (\d+)\+(\d+)\+(\d+)\+(\d+) us,.* (@\d+)?`)

	// Go 1.5 GODEBUG=gctrace=1 format
	gc15Head   = regexp.MustCompile(`^gc #(\d+) @([\d.]+)s.*:`)
	gc15Clocks = regexp.MustCompile(`^((?:\d+(?:\.\d+)?\+)*\d+(?:\.\d+)?) ms clock`)
	gc15CPUs   = regexp.MustCompile(`^((?:\d+(?:\.\d+)?[+/])*\d+(?:\.\d+)?) ms cpu`)
	gc15Ps     = regexp.MustCompile(`^(\d+) P`)
)

// NewFromLog constructs GcStats by parsing a GC log produced by
// GODEBUG=gctrace=1.
func NewFromLog(r io.Reader) (*GcStats, error) {
	log := []Phase{}
	n := 0
	haveBegin := true
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		var phases []Phase
		if gc14Log.MatchString(line) {
			var haveBegin1 bool
			phases, haveBegin1 = phasesFromLog14(scanner)
			if len(phases) != 0 {
				haveBegin = haveBegin && haveBegin1
			}
		} else if gc15Head.MatchString(line) {
			var err error
			phases, err = phasesFromLog15(scanner)
			if err != nil {
				return nil, err
			}
		}

		if len(phases) == 0 {
			continue
		}
		if haveBegin && len(log) > 0 && log[len(log)-1].Duration == -1 {
			// Update duration time of last phase
			prev := &log[len(log)-1]
			prev.Duration = phases[0].Begin - prev.Begin

			// Because of rounding, it's possible to
			// appear to have slightly overlapping cycles.
			// Scoot the cycle if this happens.
			if prev.Duration < 0 {
				delta := -prev.Duration
				if delta > int64(5*time.Millisecond) {
					return nil, fmt.Errorf("GC trace goes backward %dms between cycles %d and %d", delta/int64(time.Millisecond), prev.N, phases[0].N)
				}
				shiftPhases(phases, delta+1)
				prev.Duration += delta + 1
			}
		}

		log = append(log, phases...)
		n += 1
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Remove unterminated end phase
	if len(log) > 0 && log[len(log)-1].Duration == -1 {
		log = log[:len(log)-1]
	}

	return &GcStats{log, n, haveBegin}, nil
}

func atoi(s string) int {
	x, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return x
}

func atoi64(s string) int64 {
	x, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return x
}

func atof(s string) float64 {
	x, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(err)
	}
	return x
}

// phasesFromLog14 parses the phases for a single Go 1.4 GC cycle.
func phasesFromLog14(scanner *bufio.Scanner) (phases []Phase, haveBegin bool) {
	sub := gc14Log.FindStringSubmatch(scanner.Text())

	n := atoi(sub[1])
	stop, sweepTerm, markTerm, shrink := atoi(sub[2]), atoi(sub[3]), atoi(sub[4]), atoi(sub[5])
	var begin int64
	if sub[6] != "" {
		begin = atoi64(sub[6][1:])
		haveBegin = true
	}

	phases = []Phase{
		// Go 1.5 includes stoptheworld() in sweep termination.
		{0, int64(stop+sweepTerm) * 1000, PhaseSweepTerm, n, 1, 1, true},
		// Go 1.5 includes stack shrink in mark termination.
		{0, int64(markTerm+shrink) * 1000, PhaseMarkTerm, n, 1, 1, true},
		{0, -1, PhaseSweep, n, 1, 0, false},
	}

	if haveBegin {
		for i := range phases {
			phases[i].Begin += begin
			begin += phases[i].Duration
		}
	}

	return
}

// phasesFromLog parses the phases for a single Go 1.5 GC cycle.
func phasesFromLog15(scanner *bufio.Scanner) ([]Phase, error) {
	// TODO: Handle forced GC, too

	line := scanner.Text()
	parts := strings.SplitAfterN(line, ": ", 2)
	head := parts[0]
	parts = strings.Split(parts[1], ", ")

	sub := gc15Head.FindStringSubmatch(head)
	n, begin := atoi(sub[1]), int64(atof(sub[2])*float64(time.Second))

	var clock, cpu [5]int64
	var gomaxprocs int
	var gotClock, gotCPU, gotGomaxprocs bool

	// Process comma separated sections.
	for _, part := range parts {
		if sub = gc15Clocks.FindStringSubmatch(part); sub != nil {
			clocks := strings.Split(sub[1], "+")
			if len(clocks) != len(clock) {
				return nil, fmt.Errorf("unexpected number of clock times: %s", line)
			}
			for i, ms := range clocks {
				clock[i] = int64(atof(ms) * float64(time.Millisecond))
			}
			gotClock = true
		} else if sub = gc15CPUs.FindStringSubmatch(part); sub != nil {
			cpus := strings.Split(sub[1], "+")
			if len(cpus) != len(cpu) {
				return nil, fmt.Errorf("unexpected number of cpu times: %s", line)
			}
			for i, ms := range cpus {
				for _, ms1 := range strings.Split(ms, "/") {
					cpu[i] += int64(atof(ms1) * float64(time.Millisecond))
				}
			}
			gotCPU = true
		} else if sub = gc15Ps.FindStringSubmatch(part); sub != nil {
			gomaxprocs = atoi(sub[1])
			gotGomaxprocs = true
		}
	}

	if !gotClock || !gotCPU || !gotGomaxprocs {
		return nil, fmt.Errorf("failed to parse: %s", line)
	}

	// Create phases from raw parts.
	phases := make([]Phase, 6)
	now := begin
	for i, kind := range []PhaseKind{PhaseSweepTerm, PhaseScan, PhaseInstallWB, PhaseMark, PhaseMarkTerm} {
		stw := kind == PhaseSweepTerm || kind == PhaseMarkTerm
		var procs float64
		if clock[i] == 0 {
			if stw {
				procs = float64(gomaxprocs)
			}
		} else {
			procs = float64(cpu[i]) / float64(clock[i])
		}
		phases[i] = Phase{now, clock[i], kind, n, gomaxprocs, procs, stw}
		now += clock[i]
	}
	phases[len(phases)-1] = Phase{now, -1, PhaseSweep, n, gomaxprocs, 0, false}

	return phases, nil
}

func shiftPhases(phases []Phase, delta int64) {
	for i := range phases {
		phases[i].Begin += delta
	}
}
