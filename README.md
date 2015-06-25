This repository provides tools for computing statistics and creating
plots from garbage collection traces produced by the Go 1.4 and 1.5
runtimes. To collect such a trace, run a Go program with

    $ env GODEBUG=gctrace=1 <program>

The garbage collection trace will be written to stderr.

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
