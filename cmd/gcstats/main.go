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
	"log"
	"math"
	"os"
	"time"

	"github.com/aclements/go-gcstats/gcstats"
	"github.com/aclements/go-gcstats/internal/go-moremath/stats"
	"github.com/aclements/go-gcstats/internal/go-moremath/vec"
)

const samples = 500

var flagShow = flag.Bool("show", false, "Show plot in a window")

func main() {
	var (
		flagSummary = flag.Bool("summary", false, "Compute summary statistics")
		flagMMU     = flag.Bool("mmu", false, "Compute MMU graph")
		flagMUT     = flag.Bool("mut", false, "Compute mutator utilization topology")
		flagMUCDF   = flag.Duration("mucdf", 0, "Compute mutator utilization CDF for all windows of `duration`")
		flagMUCCDF  = flag.Duration("muccdf", 0, "Compute mutator utilization complementary CDF for all windows of `duration`")
		flagMUDMap  = flag.Bool("mudmap", false, "Compute MUD heat map")
		flagStopKDE = flag.Bool("stopkde", false, "Compute KDE of stop times")
		flagStopCDF = flag.Bool("stopcdf", false, "Compute CDF of KDE of stop times")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] [input]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if !(*flagMMU || *flagMUT || *flagMUCDF != 0 || *flagMUCCDF != 0 || *flagMUDMap || *flagStopKDE || *flagStopCDF) {
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

	if *flagMUT {
		// TOOD: Support custom percentiles
		requireProgTimes(s)
		doMUT(s)
	}

	if *flagMUCDF != 0 {
		requireProgTimes(s)
		doMUCDF(s, *flagMUCDF, "cdf")
	}

	if *flagMUCCDF != 0 {
		requireProgTimes(s)
		doMUCDF(s, *flagMUCCDF, "ccdf")
	}

	if *flagMUDMap {
		requireProgTimes(s)
		doMUDMap(s)
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

func showPlot(p *plot) {
	var err error
	if *flagShow {
		err = p.show()
	} else {
		err = p.writeTable(os.Stdout)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func doSummary(s *gcstats.GcStats) {
	// Pause time: Max, 99th %ile, 95th %ile, mean
	// Phase time distributions
	// Mutator utilization
	// 50ms mutator utilization: Min, 1st %ile, 5th %ile
	pauseTimes, _ := stopsToSamples(s)
	pauseTimes.Sort()
	fmt.Print("STW: max=", ns(pauseTimes.Percentile(1)), " 99%ile=", ns(pauseTimes.Percentile(.99)), " 95%ile=", ns(pauseTimes.Percentile(.95)), " mean=", ns(pauseTimes.Mean()), "\n")

	fmt.Println()
	clockByKind := make(map[gcstats.PhaseKind]*stats.Sample)
	for _, phase := range s.Phases() {
		if phase.Duration == -1 {
			continue
		}
		sample := clockByKind[phase.Kind]
		if sample == nil {
			sample = new(stats.Sample)
			clockByKind[phase.Kind] = sample
		}
		sample.Xs = append(sample.Xs, float64(phase.Duration))
	}
	for kind := gcstats.PhaseSweepTerm; kind <= gcstats.PhaseMultiple; kind++ {
		clock := clockByKind[kind]
		if clock == nil {
			continue
		}
		clock.Sort()
		min, max := clock.Bounds()
		if min == 0 && max == 0 {
			continue
		}
		fmt.Printf("%-10s max=%s 99%%ile=%s 95%%ile=%s mean=%s stddev=%s\n", kind.String()[5:]+":", ns(clock.Percentile(1)), ns(clock.Percentile(.99)), ns(clock.Percentile(.95)), ns(clock.Mean()), ns(clock.StdDev()))
	}

	if s.HaveProgTimes() {
		fmt.Println()
		fmt.Print("Mean mutator utilization: ", pct(s.MutatorUtilization()), "\n")
		mud := s.MutatorUtilizationDistribution(10e6)
		fmt.Print("10ms mutator utilization: min=", pct(mud.InvCDF(0)), " 1%ile=", pct(mud.InvCDF(0.01)), " 5%ile=", pct(mud.InvCDF(0.05)), "\n")
	}
}

func doMMU(s *gcstats.GcStats) {
	// 1e9 ns = 1000 ms
	windows := vec.Logspace(-3, 0, samples, 10)
	plot := newPlot("granularity", "mutator utilization", windows, "--style", "mmu")
	plot.addSeries("MMU", func(window float64) float64 {
		return s.MMU(int(window * 1e9))
	})
	showPlot(plot)
}

func doMUCDF(s *gcstats.GcStats, window time.Duration, typ string) {
	mud := s.MutatorUtilizationDistribution(int(window))
	utils := vec.Linspace(0, 1, 100)
	ylabel := "cumulative probability"
	if typ == "ccdf" {
		ylabel = "1 - cumulative probability"
	}
	plot := newPlot(fmt.Sprintf("mutator utilization at %s", window), ylabel, utils, "--style", "mud")
	plot.addSeries("", func(util float64) float64 {
		cp := mud.CDF(util)
		if typ == "ccdf" {
			cp = 1 - cp
		}
		return cp
	})
	showPlot(plot)
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
	windows := vec.Logspace(-3, 0, samples, 10)
	muds := make(map[float64]*gcstats.MUD)
	for _, window := range windows {
		muds[window] = s.MutatorUtilizationDistribution(int(window * 1e9))
	}

	plot := newPlot("granularity", "mutator utilization", windows, "--style", "mut")
	type config struct {
		label string
		x     float64
	}
	for _, c := range []config{
		{"100%ile", 0},
		{"99.9%ile", 0.001},
		{"99%ile", 0.01},
		{"90%ile", 0.1},
	} {
		plot.addSeries(c.label, func(x float64) float64 {
			return muds[x].InvCDF(c.x)
		})
	}
	showPlot(plot)
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

func jointAxis(kdes map[gcstats.PhaseKind]*stats.KDE, maxPause float64) []float64 {
	var lo, hi float64
	for i, kde := range kdes {
		if i == 0 {
			lo, hi = kde.Bounds()
		} else {
			lo1, hi1 := kde.Bounds()
			lo, hi = math.Min(lo, lo1), math.Max(hi, hi1)
		}
	}
	hi = math.Max(hi, maxPause)
	return vec.Linspace(lo, hi, samples)
}

func doStopKDE(s *gcstats.GcStats, kdes map[gcstats.PhaseKind]*stats.KDE) {
	xs := jointAxis(kdes, float64(s.MaxPause())/1e9)

	for _, kde := range kdes {
		kde.Kernel = 0
	}

	plot := newPlot("pause time", "probability density", xs, "--style", "stopkde")
	for kind := gcstats.PhaseSweepTerm; kind <= gcstats.PhaseMultiple; kind++ {
		if kde := kdes[kind]; kde != nil {
			plot.addSeries(kind.String(), kde.PDF)
		}
	}
	showPlot(plot)
}

func doStopCDF(s *gcstats.GcStats, kdes map[gcstats.PhaseKind]*stats.KDE) {
	xs := jointAxis(kdes, float64(s.MaxPause())/1e9)

	for _, kde := range kdes {
		kde.Kernel = stats.DeltaKernel
	}

	plot := newPlot("pause time", "cumulative probability", xs, "--style", "stopcdf")
	for kind := gcstats.PhaseSweepTerm; kind <= gcstats.PhaseMultiple; kind++ {
		if kde := kdes[kind]; kde != nil {
			plot.addSeries(kind.String(), kde.CDF)
		}
	}
	showPlot(plot)
}

func stopsToSamples(s *gcstats.GcStats) (all stats.Sample, byKind map[gcstats.PhaseKind]stats.Sample) {
	stops := s.Stops()
	byKind = make(map[gcstats.PhaseKind]stats.Sample)
	for _, stop := range stops {
		s := byKind[stop.Kind]
		s.Xs = append(s.Xs, float64(stop.Duration))
		byKind[stop.Kind] = s
		all.Xs = append(all.Xs, float64(stop.Duration))
	}
	return
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
