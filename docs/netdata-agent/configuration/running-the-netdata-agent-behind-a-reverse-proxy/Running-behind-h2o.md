# Running Netdata behind H2O

[H2O](https://h2o.examp1e.net/) is a new generation HTTP server that provides quicker response to users with less CPU utilization when compared to older generation of web servers.

It is notable for having much simpler configuration than many popular HTTP servers, low resource requirements, and integrated native support for many things that other HTTP servers may need special setup to use.

## Why H2O

- Sane configuration defaults mean that typical configurations are very minimalistic and easy to work with.

- Native support for HTTP/2 provides improved performance when accessing the Netdata dashboard remotely.

- Password protect access to the Netdata dashboard without requiring Netdata Cloud.

## H2O configuration file

On most systems, the H2O configuration is found under `/etc/h2o`. H2O uses [YAML 1.1](https://yaml.org/spec/1.1/), with a few special extensions, for its configuration files, with the main configuration file being `/etc/h2o/h2o.conf`.

You can edit the H2O configuration file with Nano, Vim or any other text editors with which you’re comfortable.

After making changes to the configuration files, perform the following:

- Test the configuration with `h2o -m test -c /etc/h2o/h2o.conf`

- Restart H2O to apply the changes with `/etc/init.d/h2o restart` or `service h2o restart`

## Ways to access Netdata via H2O

### As a virtual host

With this method instead of `SERVER_IP_ADDRESS:19999`, the Netdata dashboard can be accessed via a human-readable URL such as `netdata.example.com` used in the configuration below.

```yaml
hosts:
  netdata.example.com:
    listen:
      port: 80
    paths:
      /:
        proxy.preserve-host: ON
        proxy.reverse.url: http://127.0.0.1:19999
```

### As a subfolder of an existing virtual host

This method is recommended when Netdata is to be served from a subfolder (or directory).
In this case, the virtual host `netdata.example.com` already exists and Netdata has to be accessed via `netdata.example.com/netdata/`.

```yaml
hosts:
  netdata.example.com:
    listen:
      port: 80
    paths:
      /netdata:
        redirect:
          status: 301
          url: /netdata/
      /netdata/:
        proxy.preserve-host: ON
        proxy.reverse.url: http://127.0.0.1:19999
```

### As a subfolder for multiple Netdata servers, via one H2O instance

This is the recommended configuration when one H2O instance will be used to manage multiple Netdata servers via subfolders.

```yaml
hosts:
  netdata.example.com:
    listen:
      port: 80
    paths:
      /netdata/server1:
        redirect:
          status: 301
          url: /netdata/server1/
      /netdata/server1/:
        proxy.preserve-host: ON
        proxy.reverse.url: http://198.51.100.1:19999
      /netdata/server2:
        redirect:
          status: 301
          url: /netdata/server2/
      /netdata/server2/:
        proxy.preserve-host: ON
        proxy.reverse.url: http://198.51.100.2:19999
```

Of course, you can add as many backend servers as you like.

Using the above, you access Netdata on the backend servers like this:

- `http://netdata.example.com/netdata/server1/` to reach Netdata on `198.51.100.1:19999`
- `http://netdata.example.com/netdata/server2/` to reach Netdata on `198.51.100.2:19999`

### Encrypt the communication between H2O and Netdata

In case Netdata's web server has been [configured to use TLS](/src/web/server/README.md#enable-httpstls-support), it is
necessary to specify inside the H2O configuration that the final destination is using TLS. To do this, change the
`http://` on the `proxy.reverse.url` line in your H2O configuration with `https://`

### Enable authentication

Create an authentication file to enable basic authentication via H2O, this secures your Netdata dashboard.

If you don't have an authentication file, you can use the following command:

```sh
printf "yourusername:$(openssl passwd -apr1)" > /etc/h2o/passwords
```

And then add a basic authentication handler to each path definition:

```yaml
hosts:
  netdata.example.com:
    listen:
      port: 80
    paths:
      /:
        mruby.handler: |
          require "htpasswd.rb"
          Htpasswd.new("/etc/h2o/passwords", "netdata.example.com")
        proxy.preserve-host: ON
        proxy.reverse.url: http://127.0.0.1:19999
```

For more information on using basic authentication with H2O, see [their official documentation](https://h2o.examp1e.net/configure/basic_auth.html).

## Limit direct access to Netdata

If your H2O server is on `localhost`, you can use this to ensure external access is only possible through H2O:

```text
[web]
    bind to = 127.0.0.1 ::1
```

You can also use a unix domain socket. This will provide faster communication between H2O and Netdata as well:

```text
[web]
    bind to = unix:/run/netdata/netdata.sock
```

In the H2O configuration, use a line like the following to connect to Netdata via the unix socket:

```text
proxy.reverse.url http://[unix:/run/netdata/netdata.sock]
```

If your H2O server is not on localhost, you can set:

```text
[web]
    bind to = *
    allow connections from = IP_OF_H2O_SERVER
```

*note: Netdata v1.9+ support `allow connections from`*

`allow connections from` accepts [Netdata simple patterns](/src/libnetdata/simple_pattern/README.md) to match against
the connection IP address.

## Prevent the double access.log

H2O logs accesses and Netdata logs them too. You can prevent Netdata from generating its access log, by setting
this in `/etc/netdata/netdata.conf`:

```text
[logs]
    access = off
```
