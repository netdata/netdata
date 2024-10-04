# Agent-Cloud link (ACLK)

The Agent-Cloud link (ACLK) is the mechanism responsible for securely connecting a Netdata Agent to your web browser
through Netdata Cloud. The ACLK establishes an outgoing secure WebSocket (WSS) connection to Netdata Cloud on port
`443`. The ACLK is encrypted, safe, and _is only established if you connect your node_.

The Cloud App lives at app.netdata.cloud which currently resolves to the following list of IPs:

- 54.198.178.11
- 44.207.131.212
- 44.196.50.41

> **Caution**
>
>This list of IPs can change without notice, we strongly advise you to whitelist following domains `app.netdata.cloud`, `mqtt.netdata.cloud`, if this is not an option in your case always verify the current domain resolution (e.g via the `host` command).

For a guide to connecting a node using the ACLK, plus additional troubleshooting and reference information, read our [connect to Cloud
documentation](/src/claim/README.md).

## Data privacy

[Data privacy](https://netdata.cloud/privacy/) is very important to us. We firmly believe that your data belongs to
you. This is why **we don't store any metric data in Netdata Cloud**.

All the data that you see in the web browser when using Netdata Cloud, is actually streamed directly from the Netdata Agent to the Netdata Cloud dashboard. The data passes through our systems, but it isn't stored.

However, to be able to offer the stunning visualizations and advanced functionality of Netdata Cloud, it does store a limited number of _metadata_. Read more about our [security and privacy design](/docs/security-and-privacy-design/README.md).

## Enable and configure the ACLK

The ACLK is enabled by default, with its settings automatically configured and stored in the Agent's memory.

If your Agent needs to use a proxy to access the internet, you must [set up a proxy for
connecting to cloud](/src/claim/README.md).

You can configure following keys in the `netdata.conf` section `[cloud]`:

```conf
[cloud]
    statistics = yes
    query thread count = 2
```

- `statistics` enables/disables ACLK related statistics and their charts. You can disable this to save some space in the database and slightly reduce memory usage of Netdata Agent.
- `query thread count` specifies the number of threads to process cloud queries. Increasing this setting is useful for nodes with many children (streaming), which can expect to handle more queries (and/or more complicated queries).
