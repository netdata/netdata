# Metric creation

```
$ env COMPRESSION=NONE HBASE_HOME=/usr/local/hbase/ ./src/create_table.sh
$ ./build/tsdb tsd --port=4242 --staticroot=build/staticroot --cachedir=/var/cache/tsd --zkquorum=localhost:2181 --auto-metric
```

# Apache module

deflate
headers
proxy
proxy_ajp
proxy_balancer
proxy_connect
proxy_html
proxy_http
rewrite
slotmem_shm
ssl
xml2enc

# openssl

```
$ openssl req -newkey rsa:2048 -nodes -sha512 -x509 -days 365 -nodes -keyout key.pem -out cert.pem
```
