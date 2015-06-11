// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// gcstats analyzes Go garbage collection traces.
//
// To collect a GC trace, run the program with
//     $ env GODEBUG=gctrace=1 <program>
//
// gcstats supports both Go 1.4 and Go 1.5 traces; however, mutator
// utilization analyses require the following patch to the Go 1.4
// runtime to add program execution times to the trace:
//
//     --- src/runtime/mgc0.c
//     +++ src/runtime/mgc0.c
//     @@ -1484 +1484 @@
//     -				" %D(%D) handoff, %D(%D) steal, %D/%D/%D yields\n",
//     +				" %D(%D) handoff, %D(%D) steal, %D/%D/%D yields @%D\n",
//     @@ -1492 +1492 @@
//     -			stats.nprocyield, stats.nosyield, stats.nsleep);
//     +			stats.nprocyield, stats.nosyield, stats.nsleep, t0/1000);
package main

// TODO(austin): Explain analyses in doc comment.

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/aclements/go-gcstats/gcstats"
	"github.com/aclements/go-gcstats/internal/go-moremath/stats"
	"github.com/aclements/go-gcstats/internal/go-moremath/vec"
)

func main() {
	var (
		flagSummary = flag.Bool("summary", false, "Compute summary statistics")
		flagMMU     = flag.Bool("mmu", false, "Compute MMU graph")
		flagMUT     = flag.Bool("mut", false, "Compute mutator utilization topology")
		flagMUDMap  = flag.Bool("mudmap", false, "Compute MUD heat map")
		flagStopKDE = flag.Bool("stopkde", false, "Compute KDE of stop times")
		flagStopCDF = flag.Bool("stopcdf", false, "Compute CDF of KDE of stop times")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] [input]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if !(*flagMMU || *flagMUT || *flagMUDMap || *flagStopKDE || *flagStopCDF) {
		*flagSummary = true
	}

	var input io.Reader
	if flag.NArg() == 0 {
		input = os.Stdin
	} else if flag.NArg() == 1 {
		var err error
		if input, err = os.Open(flag.Arg(0)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else {
		flag.Usage()
		os.Exit(1)
	}

	// Read input log
	s, err := gcstats.NewFromLog(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing log: %s\n", err)
		os.Exit(1)
	}
	if len(s.Phases()) == 0 {
		fmt.Fprintf(os.Stderr, "no GC recorded; did you set GODEBUG=gctrace=1?")
		os.Exit(1)
	}

	if *flagSummary {
		doSummary(s)
	}

	if *flagMMU {
		requireProgTimes(s)
		doMMU(s)
	}

	if *flagMUDMap {
		requireProgTimes(s)
		doMUDMap(s)
	}

	if *flagMUT {
		// TOOD: Support custom percentiles
		requireProgTimes(s)
		doMUT(s)
	}

	if *flagStopKDE || *flagStopCDF {
		// TODO: Also plot durations of non-STW phases
		kdes := stopKDEs(s)
		if *flagStopKDE {
			doStopKDE(s, kdes)
		}
		if *flagStopCDF {
			doStopCDF(s, kdes)
		}
	}
}

func doSummary(s *gcstats.GcStats) {
	// Pause time: Max, 99th %ile, 95th %ile, mean
	// Mutator utilization
	// 50ms mutator utilization: Min, 1st %ile, 5th %ile
	pauseTimes := stats.Sample{Xs: []float64{}}
	for _, stop := range s.Stops() {
		pauseTimes.Xs = append(pauseTimes.Xs, float64(stop.Duration))
	}
	pauseTimes.Sort()
	fmt.Print("Pause times: max=", ns(pauseTimes.Percentile(1)), " 99th %ile=", ns(pauseTimes.Percentile(.99)), " 95th %ile=", ns(pauseTimes.Percentile(.95)), " mean=", ns(pauseTimes.Mean()), "\n")

	if s.HaveProgTimes() {
		fmt.Print("Mutator utilization: ", pct(s.MutatorUtilization()), "\n")

		fmt.Print("50ms mutator utilization: min=", pct(s.MMUs([]int{50000000})[0]), "\n")
	}
}

func doMMU(s *gcstats.GcStats) {
	// 1e9 ns = 1000 ms
	//windows := vec.Linspace(0, 1e9, 500)
	windows := vec.Logspace(6, 9, 500, 10)
	printTable(func(w float64) float64 {
		return s.MMU(int(w))
	}, windows)
}

func doMUDMap(s *gcstats.GcStats) {
	windows := ints(vec.Logspace(6, 9, 100, 10))
	muds := make([]*gcstats.MUD, len(windows))
	for i, windowNS := range windows {
		muds[i] = s.MutatorUtilizationDistribution(windowNS)
	}
	// gnuplot "nonuniform matrix" format
	fmt.Printf("%d ", len(windows)+1)
	for _, windowNS := range windows {
		fmt.Printf("%d ", windowNS)
	}
	fmt.Print("\n")
	utils := vec.Linspace(0, 1, 100)
	for _, util := range utils {
		fmt.Printf("%g ", util)
		for _, mud := range muds {
			fmt.Printf("%g ", mud.CDF(util))
		}
		fmt.Print("\n")
	}
}

func doMUT(s *gcstats.GcStats) {
	windows := vec.Logspace(-3, 0, 100, 10)
	fmt.Printf("granularity\t100%%ile\t99.9%%ile\t99%%ile\t90%%ile\n")
	for _, window := range windows {
		mud := s.MutatorUtilizationDistribution(int(window * 1e9))
		fmt.Printf("%g\t%g\t%g\t%g\t%g\n", window, mud.InvCDF(0), mud.InvCDF(0.001), mud.InvCDF(0.01), mud.InvCDF(0.1))
	}
}

func stopKDEs(s *gcstats.GcStats) map[gcstats.PhaseKind]*stats.KDE {
	stops := s.Stops()
	times := make(map[gcstats.PhaseKind]stats.Sample)
	for _, stop := range stops {
		s := times[stop.Kind]
		s.Xs = append(s.Xs, float64(stop.Duration)/1e9)
		times[stop.Kind] = s
	}

	kdes := make(map[gcstats.PhaseKind]*stats.KDE)
	for kind, sample := range times {
		// XXX Bandwidth
		kdes[kind] = &stats.KDE{
			Sample: sample,
			//Bandwidth:      stats.FixedBandwidth(100000),
			BoundaryMethod: stats.BoundaryReflect,
			BoundaryMax:    math.Inf(1),
		}
	}
	return kdes
}

func jointAxis(kdes map[gcstats.PhaseKind]*stats.KDE) []float64 {
	var lo, hi float64
	for i, kde := range kdes {
		if i == 0 {
			lo, hi = kde.Bounds()
		} else {
			lo1, hi1 := kde.Bounds()
			lo, hi = math.Min(lo, lo1), math.Max(hi, hi1)
		}
	}
	return vec.Linspace(lo, hi, 100)
}

func kdeHeader(kdes map[gcstats.PhaseKind]*stats.KDE) {
	fmt.Printf("pause time")
	for kind := gcstats.PhaseSweepTerm; kind <= gcstats.PhaseMultiple; kind++ {
		if kdes[kind] != nil {
			fmt.Printf("\t%s", kind)
		}
	}
	fmt.Printf("\n")
}

func doStopKDE(s *gcstats.GcStats, kdes map[gcstats.PhaseKind]*stats.KDE) {
	xs := jointAxis(kdes)
	kdeHeader(kdes)

	for _, kde := range kdes {
		kde.Kernel = 0
	}

	for _, x := range xs {
		fmt.Printf("%v", x)
		for kind := gcstats.PhaseSweepTerm; kind <= gcstats.PhaseMultiple; kind++ {
			if kde := kdes[kind]; kde != nil {
				fmt.Printf("\t%v", kde.PDF(x))
			}
		}
		fmt.Printf("\n")
	}
}

func doStopCDF(s *gcstats.GcStats, kdes map[gcstats.PhaseKind]*stats.KDE) {
	xs := jointAxis(kdes)
	kdeHeader(kdes)

	for _, kde := range kdes {
		kde.Kernel = stats.DeltaKernel
	}

	for _, x := range xs {
		fmt.Printf("%v", x)
		for kind := gcstats.PhaseSweepTerm; kind <= gcstats.PhaseMultiple; kind++ {
			if kde := kdes[kind]; kde != nil {
				fmt.Printf("\t%v", kde.CDF(x))
			}
		}
		fmt.Printf("\n")
	}
}

func ints(xs []float64) []int {
	ys := make([]int, len(xs))
	for i, x := range xs {
		ys[i] = int(x)
	}
	return ys
}

func printTable(f func(float64) float64, xs []float64) {
	for _, x := range xs {
		fmt.Println(x, f(x))
	}
}

func requireProgTimes(s *gcstats.GcStats) {
	if !s.HaveProgTimes() {
		fmt.Fprintln(os.Stderr,
			"This analysis requires program execution times, which are missing from\n"+
				"this GC trace. Please see 'go doc gcstats' for how to enable these.")
		os.Exit(1)
	}
}
