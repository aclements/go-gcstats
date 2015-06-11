// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "fmt"

func ns(ns float64) string {
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

func pct(x float64) string {
	return fmt.Sprintf("%.2g%%", 100*x)
}
