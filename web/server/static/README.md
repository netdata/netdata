# `static-threaded` web server

The `static-threaded` web server spawns a fixed number of threads.
All the threads are concurrently listening for web requests on the same sockets.
The kernel distributes the incoming requests to them.

Each thread uses non-blocking I/O so it can serve any number of web requests in parallel.

This web server respects the `keep-alive` HTTP header to serve multiple HTTP requests via the same connection. 