This repository provides tools for computing statistics and creating
plots from garbage collection traces produced by the Go 1.4 and 1.5
runtimes. To collect such a trace, run a Go program with

    $ env GODEBUG=gctrace=1 <program>

The garbage collection trace will be written to stderr.

plot-gctrace
------------

`cmd/plot-gctrace` plots the breakdown of GC CPU utilization and heap
size versus wall-clock time. `plot-gctrace` is a Python script, so it
can be executed directly.

![plot-gctrace output](/media/plot-gctrace.png)

gcstats
-------

`cmd/gcstats` computes and plots a variety of high-level statistics
from a GC trace. To install it, run

    $ go get github.com/aclements/go-gcstats/cmd/gcstats

By default, `gcstats` prints a high-level summary of the pause time
distribution and the mutator utilization,

    $ gcstats < gctrace
    Pause times: max=1ms 99%ile=1ms 95%ile=929us mean=399us
    Mean mutator utilization: 89%
    10ms mutator utilization: min=51% 1%ile=55% 5%ile=55%

It can also plot the pause time distribution as a kernel density
estimate or empirical CDF:

    $ gcstats -stopcdf -show < gctrace
![gcstats -stopcdf output](/media/stopcdf.png)

However, the pause time distribution doesn't show you how close
together the pauses are (all pauses could be under a millisecond, but
if they're only a microsecond apart, that's still bad!). Hence,
`gcstats` can also plot a *mutator utilization topology*, which shows
mutator utilization at various percentiles and over windows of time
ranging from 1ms to 1s.

    $ gcstats -mut -show < gctrace
![gcstats -mut output](/media/mut.png)

Go 1.4
------

Note that some of the analyses require a small patch to the Go 1.4
runtime to add program execution times. To apply this patch to a
Go 1.4 tree, run

    $ cd $GOROOT
    $ patch < $GOPATH/src/github.com/aclements/go-gcstats/go14.patch

Dependencies for plotting
-------------------------

The plotting facilities of these tools currently depend on Python 3
and [matplotlib](http://matplotlib.org/). In addition,
[Seaborn](http://stanford.edu/~mwaskom/software/seaborn/) is
recommended.

To install these packages on Debian and Ubuntu, use

    apt-get install python3-matplotlib python3-scipy python3-pandas
    pip3 install seaborn

For general instructions on installing Seaborn, see [installing
Seaborn](http://stanford.edu/~mwaskom/software/seaborn/installing.html).
