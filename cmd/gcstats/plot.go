// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/aclements/go-gcstats/internal/go-moremath/vec"
)

//go:generate sh -c "(echo '// GENERATED. DO NOT EDIT.'; echo; echo package main; echo; echo -n 'var plotpy = `'; cat plot.py; echo '`') > bindata_plotpy.go"

type plot struct {
	hdrs []string
	cols [][]float64
	args []string
}

func newPlot(xlabel string, xs []float64, args ...string) *plot {
	return &plot{[]string{xlabel}, [][]float64{xs}, args}
}

func (p *plot) addSeries(label string, f func(float64) float64) {
	p.hdrs = append(p.hdrs, label)
	p.cols = append(p.cols, vec.Map(f, p.cols[0]))
}

func (p *plot) show() error {
	f, err := ioutil.TempFile("", "gcstats")
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(f.Name())

	if err := f.Chmod(0700); err != nil {
		return err
	}
	if _, err := f.Write([]byte(plotpy)); err != nil {
		return err
	}
	f.Close()

	cmd := exec.Command(f.Name(), p.args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := p.writeTable(stdin); err != nil {
		return err
	}
	if err := stdin.Close(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func (p *plot) writeTable(w io.Writer) error {
	for i, hdr := range p.hdrs {
		if i != 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, hdr)
	}
	fmt.Fprint(w, "\n")

	for row := range p.cols[0] {
		for i, col := range p.cols {
			if i != 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, col[row])
		}
		fmt.Fprint(w, "\n")
	}

	return nil
}
