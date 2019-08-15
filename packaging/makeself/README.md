# Netdata static binary build

To build the static binary 64-bit distribution package, run:

```bash
cd /path/to/netdata.git
./packaging/makeself/build-x86_64-static.sh
```

The program will:

1.  setup a new docker container with Alpine Linux
2.  install the required alpine packages (the build environment, needed libraries, etc)
3.  download and compile third party apps that are packaged with Netdata (`bash`, `curl`, etc)
4.  compile Netdata

Once finished, a file named `netdata-vX.X.X-gGITHASH-x86_64-DATE-TIME.run` will be created in the current directory. This is the Netdata binary package that can be run to install Netdata on any other computer.

---

## building binaries with debug info

To build Netdata binaries with debugging / tracing information in them, use:

```bash
cd /path/to/netdata.git
./packaging/makeself/build-x86_64-static.sh debug
```

These binaries are not optimized (they are a bit slower), they have certain features disables (like log flood protection), other features enables (like `debug flags`) and are not stripped (the binary files are bigger, since they now include source code tracing information).

### debugging Netdata binaries

Once you have installed a binary package with debugging info, you will need to install `valgrind` and run this command to start Netdata:

```bash
PATH="/opt/netdata/bin:${PATH}" valgrind --undef-value-errors=no /opt/netdata/bin/srv/netdata -D
```

The above command, will run Netdata under `valgrind`. While Netdata runs under `valgrind` it will be 10x slower and use a lot more memory.

If Netdata crashes, `valgrind` will print a stack trace of the issue. Open a github issue to let us know.

To stop Netdata while it runs under `valgrind`, press Control-C on the console.

> If you omit the parameter `--undef-value-errors=no` to valgrind, you will get hundreds of errors about conditional jumps that depend on uninitialized values. This is normal. Valgrind has heuristics to prevent it from printing such errors for system libraries, but for the static Netdata binary, all the required libraries are built into Netdata. So, valgrind cannot appply its heuristics and prints them.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fmakeself%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
