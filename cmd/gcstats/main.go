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
		summary    bool
		mmu        bool
		mudmap     bool
		mudpctiles bool
		stopkde    bool
		stopcdf    bool
		input      io.Reader
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] [input]\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.BoolVar(&summary, "summary", false, "Compute summary statistics")
	flag.BoolVar(&mmu, "mmu", false, "Compute MMU graph")
	flag.BoolVar(&mudmap, "mudmap", false, "Compute MUD heat map")
	flag.BoolVar(&mudpctiles, "mudpctiles", false, "Compute MUD 0, 0.1, 1, and 10th percentiles")
	flag.BoolVar(&stopkde, "stopkde", false, "Compute KDE of stop times")
	flag.BoolVar(&stopcdf, "stopcdf", false, "Compute CDF of KDE of stop times")
	flag.Parse()

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
		fmt.Fprintf(os.Stderr, "no GC recorded; did you enable GC tracing?")
		os.Exit(1)
	}

	if summary {
		// Pause time: Max, 99th %ile, 95th %ile, mean
		// Mutator utilization
		// 50ms mutator utilization: Min, 1st %ile, 5th %ile
		pauseTimes := stats.Sample{Xs: []float64{}}
		for _, stop := range s.Stops() {
			pauseTimes.Xs = append(pauseTimes.Xs, float64(stop.Duration))
		}
		pauseTimes.Sort()
		fmt.Print("Pause times: max=", NS(pauseTimes.Percentile(1)), " 99th %ile=", NS(pauseTimes.Percentile(.99)), " 95th %ile=", NS(pauseTimes.Percentile(.95)), " mean=", NS(pauseTimes.Mean()), "\n")

		if s.HaveProgTimes() {
			fmt.Print("Mutator utilization: ", Pct(s.MutatorUtilization()), "\n")

			fmt.Print("50ms mutator utilization: min=", Pct(s.MMUs([]int{50000000})[0]), "\n")
		}
	}

	if mmu {
		requireProgTimes(s)
		// 1e9 ns = 1000 ms
		//windows := ints(vec.Linspace(0, 1e9, 500))
		windows := ints(vec.Logspace(6, 9, 500, 10))
		mmu := s.MMUs(windows)
		for i := 0; i < len(windows); i++ {
			fmt.Println(windows[i], mmu[i])
		}
	}

	if mudmap {
		requireProgTimes(s)
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

	if mudpctiles {
		requireProgTimes(s)
		windows := ints(vec.Logspace(6, 9, 100, 10))
		for _, windowNS := range windows {
			mud := s.MutatorUtilizationDistribution(windowNS)
			fmt.Printf("%d %g %g %g %g\n", windowNS, mud.InvCDF(0), mud.InvCDF(0.001), mud.InvCDF(0.01), mud.InvCDF(0.1))
		}
	}

	if stopkde || stopcdf {
		// TODO: Also plot durations of non-STW phases

		stops := s.Stops()
		times := make(map[gcstats.PhaseKind]stats.Sample)
		for _, stop := range stops {
			s := times[stop.Kind]
			s.Xs = append(s.Xs, float64(stop.Duration))
			times[stop.Kind] = s
		}

		for kind := gcstats.PhaseSweepTerm; kind <= gcstats.PhaseMultiple; kind++ {
			sample := times[kind]

			// for kind, sample := range times {
			if sample.Xs == nil {
				continue
			}

			// XXX Bandwidth
			kde := stats.KDE{
				Sample: sample,
				//Bandwidth:      stats.FixedBandwidth(100000),
				BoundaryMethod: stats.BoundaryReflect,
				BoundaryMax:    math.Inf(1),
			}
			if stopcdf {
				kde.Kernel = stats.DeltaKernel
			}
			lo, hi := kde.Bounds()
			hi = math.Max(hi, float64(s.MaxPause()))
			if stopkde {
				fmt.Printf("PDF \"%s\"\n", kind)
				printTable(kde.PDF, vec.Linspace(lo, hi, 100))
				fmt.Printf("\n\n")
			}
			if stopcdf {
				fmt.Printf("CDF \"%s\"\n", kind)
				printTable(kde.CDF, vec.Linspace(lo, hi, 100))
				fmt.Printf("\n\n")
			}
		}
	}
}

func NS(ns float64) string {
	for _, d := range []struct {
		unit string
		div  float64
	}{{"ns", 1000}, {"us", 1000}, {"ms", 1000}, {"sec", 60}, {"min", 60}, {"hour", 0}} {
		if ns < d.div || d.div == 0 {
			return fmt.Sprintf("%d%s", int64(ns), d.unit)
		}
		ns /= d.div
	}
	panic("not reached")
}

func Pct(x float64) string {
	return fmt.Sprintf("%.2g%%", 100*x)
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
