# MongoDB backend

## Prerequisites

To use MongoDB as a backend, `libmongoc` 1.7.0 or higher should be
[installed](http://mongoc.org/libmongoc/current/installing.html) first. Next, Netdata should be re-installed from the
source. The installer will detect that the required libraries are now available.

## Configuration

To enable data sending to the MongoDB backend set the following options in `netdata.conf`:

```conf
[backend]
    enabled = yes
    type = mongodb
```

In the Netdata configuration directory run `./edit-config mongodb.conf` and set [MongoDB
URI](https://docs.mongodb.com/manual/reference/connection-string/), database name, and collection name:

```yaml
# URI
uri = mongodb://<hostname>

# database name
database = your_database_name

# collection name
collection = your_collection_name
```

The default socket timeout depends on the backend update interval. The timeout is 500 ms shorter than the interval (but
not less than 1000 ms). You can alter the timeout using the `sockettimeoutms` MongoDB URI option.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fbackends%2Fmongodb%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
