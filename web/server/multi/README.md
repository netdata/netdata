# `multi-threaded` web server

The `multi-threaded` web server spawns a thread for each connection it receives.

Each thread uses non-blocking I/O so it can serve any number of web requests in parallel,
though this is not supported by HTTP, so in practice each thread serves all the requests sequentially.

Each thread respects the `keep-alive` HTTP header to serve multiple HTTP requests via the same connection. 
[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fweb%2Fserver%2Fmulti%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
