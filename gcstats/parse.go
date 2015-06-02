// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
)

var (
	// Go 1.4 GODEBUG=gctrace=1 format, with optional start time
	gc14Log = regexp.MustCompile(`^gc(\d+)\(\d+\): (\d+)\+(\d+)\+(\d+)\+(\d+) us,.* (@\d+)?`)

	// Go 1.5 runtime.GCstarttimes format
	gcLog      = regexp.MustCompile(`^GC: #(\d+)\s+\d+ns\s+@(\d+)\s.*gomaxprocs=(\d+)`)
	phaseLog   = regexp.MustCompile(`^GC:\s+([a-z ]+):\s+(\d+)ns\s.*procs=([-+]?\d*\.?\d+(?:[eE][-+]?\d+)?)`)
	phaseNames = map[string]PhaseKind{
		"sweep term": PhaseSweepTerm,
		"scan":       PhaseScan,
		"install wb": PhaseInstallWB,
		"mark":       PhaseMark,
		"mark term":  PhaseMarkTerm,
	}
)

// NewFromLog constructs GcStats by parsing a GC log produced by
// runtime.GCstarttimes(2).
func NewFromLog(r io.Reader) (*GcStats, error) {
	log := []Phase{}
	n := 0
	haveBegin := true
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
	skipScan:
		line := scanner.Text()

		var phases []Phase
		var nextLine bool
		if gc14Log.MatchString(line) {
			var haveBegin1 bool
			phases, haveBegin1 = phasesFromLog14(scanner)
			if len(phases) != 0 {
				haveBegin = haveBegin && haveBegin1
			}
		} else if gcLog.MatchString(line) {
			// phasesFromLog may consume the following line
			phases, nextLine = phasesFromLog(scanner)
		}

		if phases != nil {
			if len(log) > 0 && log[len(log)-1].End == -1 {
				// Update end time of last phase
				log[len(log)-1].End = phases[0].Begin
			}

			log = append(log, phases...)
			n += 1
		}
		if nextLine {
			goto skipScan
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Remove unterminated end phase
	if len(log) > 0 && log[len(log)-1].End == -1 {
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
		Phase{0, -1, PhaseSweep, n, 1, 0, false},
	}

	if haveBegin {
		for i := range phases {
			phases[i].Begin += begin
			phases[i].End += begin
			begin = phases[i].End
		}
	}

	return
}

// phasesFromLog parses the phases for a single GC cycle.
func phasesFromLog(scanner *bufio.Scanner) ([]Phase, bool) {
	// Create implicit first sweep phase
	var phases []Phase

	// Parse leading GC line
	sub := gcLog.FindStringSubmatch(scanner.Text())
	n, time, gomaxprocs := atoi(sub[1]), atoi64(sub[2]), atoi(sub[3])

	// Parse phase times
	for scanner.Scan() {
		sub := phaseLog.FindStringSubmatch(scanner.Text())
		if sub == nil {
			break
		}
		kind, ok := phaseNames[sub[1]]
		if !ok {
			fmt.Fprintln(os.Stderr, "unknown GC phase", sub[1])
			continue
		}
		dur, _ := strconv.Atoi(sub[2])
		procs, err := strconv.ParseFloat(sub[3], 64)
		if err != nil {
			// TODO: Should this be a real error?
			fmt.Fprintln(os.Stderr, "bad procs =", sub[3])
			continue
		}

		phases = append(phases, Phase{
			Begin:      time,
			End:        time + int64(dur),
			Kind:       kind,
			N:          n,
			Gomaxprocs: gomaxprocs,
			GCProcs:    procs,
			STW:        procs == float64(gomaxprocs),
		})

		time += int64(dur)
	}

	// sweep is implicitly the last phase
	phases = append(phases, Phase{
		Begin:      time,
		End:        -1,
		Kind:       PhaseSweep,
		N:          n,
		Gomaxprocs: gomaxprocs,
		GCProcs:    0,
		STW:        false,
	})

	if scanner.Err() != nil {
		return nil, false
	}
	if len(phases) != 6 {
		fmt.Fprintln(os.Stderr, "missing GC phases in cycle", n, "; expected 6, got", len(phases))
		return nil, true
	}

	return phases, true
}
