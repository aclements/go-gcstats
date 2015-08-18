# Copyright 2015 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

import sys
import re

heapMinimum = 4<<20

def parse(fp, GOGC=None, omit_forced=True):
    """Parse gctrace output, yielding a series of Rec objects.

    If GOGC is not None, records will include a computed H_g."""

    r = re.compile(r'gc #?(?P<n>[0-9]+) @(?P<start>[0-9.]+)s (?P<util>[0-9]+)%: '
                   r'(?P<parts>.*)')
    H_m_prev = None
    for line in fp:
        m = r.match(line)
        if not m:
            continue
        d = m.groupdict()
        d['n'] = int(d['n'])
        d['start'] = _sec(d['start'])
        d['util'] = int(d['util']) / 100
        d['forced'] = '(forced)' in line

        # Process parts
        for part in d.pop('parts').split(','):
            part = part.strip()

            m = re.match(r'([+0-9.]+) ms clock', part)
            if m:
                v = list(map(_ms, m.group(1).split('+')))
                d['clocks'] = v
                d['clocksSTW'] = v[::2]
                d['clocksCon'] = v[1::2]
                continue

            m = re.match(r'([+/0-9.]+) ms cpu', part)
            if m:
                phases = m.group(1).split('+')
                d['markPhase'] = 3
                mark = phases[d['markPhase']]
                if '/' in mark:
                    d['cpu_assist'], d['cpu_bg'], d['cpu_idle'] \
                        = map(_ms, mark.split('/'))
                    phases[3] = (d['cpu_assist'] + d['cpu_bg']) * 1e3
                else:
                    d['cpu_bg'] = _ms(mark)
                    d['cpu_assist'] = d['cpu_idle'] = 0
                v = list(map(_ms, phases))
                d['cpus'] = v
                d['stw'] = [False] * len(v)
                d['stw'][0] = d['stw'][-1] = True
                d['cpusSTW'] = sum(t for t, stw in zip(v, d['stw']) if stw)
                d['cpusCon'] = sum(t for t, stw in zip(v, d['stw']) if not stw)
                continue

            m = re.match(r'(?P<H_T>[0-9]+)->(?P<H_a>[0-9]+)->(?P<H_m>[0-9]+) MB', part)
            if m:
                for k, v in m.groupdict().items():
                    v = int(v) << 20
                    d[k] = v
                continue

            m = re.match(r'([0-9]+) MB goal', part)
            if m:
                d['H_g'] = int(m.group(1)) << 20
                continue

            m = re.match(r'([0-9]+) P', part)
            if m:
                d['gomaxprocs'] = int(m.group(1))
                continue

            print('unexpected part in GC line: %s' % part, file=sys.stderr)

        # Compute derived fields
        d['end'] = d['start'] + sum(d['clocks'])
        if GOGC is not None and 'H_g' not in d:
            # Compute H_g ourselves
            if H_m_prev is None:
                d['H_g'] = heapMinimum
            else:
                d['H_g'] = int(H_m_prev * (1 + GOGC/100))
        H_m_prev = d['H_m']
        if not omit_forced or not d['forced']:
            yield Rec(d)

def _num(x):
    if not isinstance(x, str):
        return x
    try:
        return int(x)
    except ValueError:
        return float(x)

def _ms(x):
    return _num(x) / 1e3

def _sec(x):
    return _num(x)

class Rec:
    def __init__(self, dct):
        for k, v in dct.items():
            setattr(self, k, v)
