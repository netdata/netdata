
# SMA Sunny Webbox

[SMA Sunny Webbox](http://files.sma.de/dl/4253/WEBBOX-DUS131916W.pdf)

Example netdata configuration for node.d/sma_webbox.conf

The module supports any number of name servers, like this:

```json
{
    "enable_autodetect": false,
    "update_every": 5,
    "servers": [
        {
            "name": "plant1",
            "hostname": "10.0.1.1",
            "update_every": 10
        },
        {
            "name": "plant2",
            "hostname": "10.0.2.1",
            "update_every": 15
        }
    ]
}
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fnode.d.plugin%2Fsma_webbox%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
