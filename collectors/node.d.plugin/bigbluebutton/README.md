<!--
title: "BigBlueButton monitoring with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/node.d.plugin/bigbluebutton/README.md
sidebar_label: "BigBlueButton"
-->

# BigBlueButton monitoring with Netdata

Example Netdata configuration for node.d/bigbluebutton.conf

The module supports any number of servers, like this.
Auto detection is not supported.

```json
{
    "enable_autodetect": false,
    "update_every": 5,
    "servers": [
        {
            "name": "bbb",
            "url": "https://localhost/api",
            "update_every": 10,
            "secret": "Your_BBB_SECRET"
        }
    ]
}
```

To get the `url` and the `secret` configuration parameter, you can run `bbb-conf --secret` on your BigBlueButton server.
