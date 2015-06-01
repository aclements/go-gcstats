// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gcstats

func int64Min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func int64Max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
