
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
