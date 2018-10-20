# `single-threaded` web server

The `single-threaded` web server runs as a single thread inside netdata.
It uses non-blocking I/O so it can serve any number of web requests in parallel.

This web server respects the `keep-alive` HTTP header to serve multiple HTTP requests via the same connection.