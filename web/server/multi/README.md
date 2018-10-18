# `multi-threaded` web server

The `multi-threaded` web server spawns a thread for each connection it receives.

Each thread uses non-blocking I/O so it can serve any number of web requests in parallel,
though this is not supported by HTTP, so in practice each thread serves all the requests sequentially.

Each thread respects the `keep-alive` HTTP header to serve multiple HTTP requests via the same connection. 