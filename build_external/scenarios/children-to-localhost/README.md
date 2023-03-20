# Stream children to localhost

1. Run `docker-compose up --scale=50`
2. Copy `parent_stream.conf` to the `stream.conf` of a local agent
3. Restart the local agent

You'll have 50 child agents streaming to the parent agent that runs locally.

Useful for easily stress testing, restarting, profiling, debugging, etc, a
locally-built agent during development.
