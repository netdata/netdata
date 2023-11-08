### Understand the alert

The `redis_connections_rejected` alert is triggered when the number of connections rejected by Redis due to the `maxclients` limit being reached in the last minute is greater than 0. This means that Redis is no longer able to accept new connections as it has reached its maximum allowed clients.

### What does maxclients limit mean?

The `maxclients` limit in Redis is the maximum number of clients that can be connected to the Redis instance at the same time. When the Redis server reaches its `maxclients` limit, any new connection attempts will be rejected.

### Troubleshoot the alert

1. Check the current number of connections in Redis:

   Use the `redis-cli` command-line tool to check the current number of clients connected to the Redis server:

   ```
   redis-cli client list | wc -l
   ```

2. Check Redis configuration file for the maxclients setting:

   The `maxclients` value can be found in the Redis configuration file, usually called `redis.conf`. Open the file and search for `maxclients` to find the current limit.

   ```
   grep 'maxclients' /etc/redis/redis.conf
   ```

3. Increase the maxclients limit.

   If necessary, increase the `maxclients` limit in the Redis configuration file (`redis.conf`), and then restart the Redis service to apply the changes:

   ```
   sudo systemctl restart redis
   ```

   _**Note**: Keep in mind that increasing the `maxclients` limit might cause increased memory consumption._

4. Inspect client connections.

   Determine if the connections are legitimate and needed for your application's requirements, or if some clients are connecting unnecessarily. Optimize your application or services as needed to reduce the number of unwanted connections.

5. Monitor connection usage.

   Keep an eye on connection usage over time to better understand the trends and patterns in your system, and adjust the `maxclients` configuration accordingly.

### Useful resources

1. [Redis Clients documentation](https://redis.io/topics/clients)
2. [Redis configuration documentation](https://redis.io/topics/config)
