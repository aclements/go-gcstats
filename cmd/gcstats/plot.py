#!/usr/bin/env python3
# -*- python -*-

# Copyright 2015 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import sys
import argparse

import numpy as np
import matplotlib as mpl
mpl.use('GTK3Cairo')
mpl.rc('figure', facecolor='1')
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
try:
    import seaborn as sns
except ImportError as e:
    print('error importing seaborn: %s\nPlease pip3 install seaborn.' % e,
          file=sys.stderr)
    sns = None

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--style', choices=('mmu', 'mut', 'stopdist', 'mud'),
                        help='Plot style', required=True)
    parser.add_argument('--ylabel', help='Y axis label')
    args = parser.parse_args()

    rows = [line.strip('\n').split('\t') for line in sys.stdin]
    table = [[col[0]] + list(map(float, col[1:])) for col in zip(*rows)]

    if args.style == 'mut':
        # Continuous palette (has to happen before plt.subplots)
        if sns is not None:
            sns.set_palette("Blues_r")

    fig, ax = plt.subplots(1, 1)

    if args.style in ('mmu', 'mut'):
        ax.set_xscale('log')

    if args.style in ('mmu', 'mut', 'stopdist', 'mud'):
        ax.set_ylim(bottom=0, top=1)

    if args.style in ('mmu', 'mut', 'stopdist'):
        ax.xaxis.set_major_formatter(tickerSec)

    ax.set_xlabel(table[0][0])
    if args.ylabel:
        ax.set_ylabel(args.ylabel)

    for col in table[1:]:
        ax.plot(table[0][1:], col[1:], label=col[0])
    ax.legend(loc='best')

    if args.style == 'mut':
        # Reverse legend order
        handles, labels = ax.get_legend_handles_labels()
        ax.legend(handles[::-1], labels[::-1], loc='best')

    plt.show()

def prettySec(x):
    if x == 0:
        return '0s'
    neg = ''
    if x < 0:
        neg = '-'
        x = -x
    units = (('ns', 1e-9), ('\u00B5s', 1e-6), ('ms', 1e-3), ('s', 1))
    unit = units[0]
    for u in units:
        if u[1] > x:
            break
        unit = u
    s = ('%.3f' % (x / unit[1])).rstrip('0').rstrip('.')
    return neg + s + unit[0]
tickerSec = ticker.FuncFormatter(lambda x, pos: prettySec(x))

if __name__ == '__main__':
    main()
