# ISC Bind Statistics

Using this netdata collector, you can monitor one or more ISC Bind servers.

The source code for this plugin in [here](https://github.com/netdata/netdata/blob/master/node.d/named.node.js).

## Example netdata charts

Depending on the number of views your bind has, you may get a large number of charts.
Here this is with just one view:

![image](https://cloud.githubusercontent.com/assets/2662304/12765473/879b8e04-ca07-11e5-817d-b0651996c42b.png)
![image](https://cloud.githubusercontent.com/assets/2662304/12766538/12b272fa-ca0d-11e5-81e1-6a9f8ff488ff.png)

## How it works

The plugin will execute (from within node.js) the equivalent of:

```sh
curl "http://localhost:8888/json/v1/server"
```

Here is a sample of the output this command produces.

```js
{
  "json-stats-version":"1.0",
  "boot-time":"2016-01-31T08:20:48Z",
  "config-time":"2016-01-31T09:28:03Z",
  "current-time":"2016-02-02T22:22:20Z",
  "opcodes":{
    "QUERY":247816,
    "IQUERY":0,
    "STATUS":0,
    "RESERVED3":0,
    "NOTIFY":0,
    "UPDATE":3813,
    "RESERVED6":0,
    "RESERVED7":0,
    "RESERVED8":0,
    "RESERVED9":0,
    "RESERVED10":0,
    "RESERVED11":0,
    "RESERVED12":0,
    "RESERVED13":0,
    "RESERVED14":0,
    "RESERVED15":0
  },
  "qtypes":{
    "A":89519,
    "NS":863,
    "CNAME":1,
    "SOA":1,
    "PTR":116779,
    "MX":276,
    "TXT":198,
    "AAAA":39324,
    "SRV":850,
    "ANY":5
  },
  "nsstats":{
    "Requestv4":251630,
    "ReqEdns0":1255,
    "ReqTSIG":3813,
    "ReqTCP":57,
    "AuthQryRej":1455,
    "RecQryRej":122,
    "Response":245918,
    "TruncatedResp":44,
    "RespEDNS0":1255,
    "RespTSIG":3813,
    "QrySuccess":205159,
    "QryAuthAns":119495,
    "QryNoauthAns":120770,
    "QryNxrrset":32711,
    "QrySERVFAIL":262,
    "QryNXDOMAIN":2395,
    "QryRecursion":40885,
    "QryDuplicate":5712,
    "QryFailure":1577,
    "UpdateDone":2514,
    "UpdateFail":1299,
    "UpdateBadPrereq":1276,
    "QryUDP":246194,
    "QryTCP":45,
    "OtherOpt":101
  },
  "views":{
    "local":{
      "resolver":{
        "stats":{
          "Queryv4":74577,
          "Responsev4":67032,
          "NXDOMAIN":601,
          "SERVFAIL":5,
          "FORMERR":7,
          "EDNS0Fail":7,
          "Truncated":3071,
          "Lame":4,
          "Retry":11826,
          "QueryTimeout":1838,
          "GlueFetchv4":6864,
          "GlueFetchv4Fail":30,
          "QryRTT10":112,
          "QryRTT100":42900,
          "QryRTT500":23275,
          "QryRTT800":534,
          "QryRTT1600":97,
          "QryRTT1600+":20,
          "BucketSize":31,
          "REFUSED":13
        },
        "qtypes":{
          "A":64931,
          "NS":870,
          "CNAME":185,
          "PTR":5,
          "MX":49,
          "TXT":149,
          "AAAA":7972,
          "SRV":416
        },
        "cache":{
          "A":40356,
          "NS":8032,
          "CNAME":14477,
          "PTR":2,
          "MX":21,
          "TXT":32,
          "AAAA":3301,
          "SRV":94,
          "DS":237,
          "RRSIG":2301,
          "NSEC":126,
          "!A":52,
          "!NS":4,
          "!TXT":1,
          "!AAAA":3797,
          "!SRV":9,
          "NXDOMAIN":590
        },
        "cachestats":{
          "CacheHits":1085188,
          "CacheMisses":109,
          "QueryHits":464755,
          "QueryMisses":55624,
          "DeleteLRU":0,
          "DeleteTTL":42615,
          "CacheNodes":5188,
          "CacheBuckets":2079,
          "TreeMemTotal":2326026,
          "TreeMemInUse":1508075,
          "HeapMemMax":132096,
          "HeapMemTotal":393216,
          "HeapMemInUse":132096
        },
        "adb":{
          "nentries":1021,
          "entriescnt":3157,
          "nnames":1021,
          "namescnt":3022
        }
      }
    },
    "public":{
      "resolver":{
        "stats":{
          "BucketSize":31
        },
        "qtypes":{
        },
        "cache":{
        },
        "cachestats":{
          "CacheHits":0,
          "CacheMisses":0,
          "QueryHits":0,
          "QueryMisses":0,
          "DeleteLRU":0,
          "DeleteTTL":0,
          "CacheNodes":0,
          "CacheBuckets":64,
          "TreeMemTotal":287392,
          "TreeMemInUse":29608,
          "HeapMemMax":1024,
          "HeapMemTotal":262144,
          "HeapMemInUse":1024
        },
        "adb":{
          "nentries":1021,
          "nnames":1021
        }
      }
    },
    "_bind":{
      "resolver":{
        "stats":{
          "BucketSize":31
        },
        "qtypes":{
        },
        "cache":{
        },
        "cachestats":{
          "CacheHits":0,
          "CacheMisses":0,
          "QueryHits":0,
          "QueryMisses":0,
          "DeleteLRU":0,
          "DeleteTTL":0,
          "CacheNodes":0,
          "CacheBuckets":64,
          "TreeMemTotal":287392,
          "TreeMemInUse":29608,
          "HeapMemMax":1024,
          "HeapMemTotal":262144,
          "HeapMemInUse":1024
        },
        "adb":{
          "nentries":1021,
          "nnames":1021
        }
      }
    }
  }
}
```


From this output it collects:

- Global Received Requests by IP version (IPv4, IPv6)
- Global Successful Queries
- Current Recursive Clients
- Global Queries by IP Protocol (TCP, UDP)
- Global Queries Analysis
- Global Received Updates
- Global Query Failures
- Global Query Failures Analysis
- Other Global Server Statistics
- Global Incoming Requests by OpCode
- Global Incoming Requests by Query Type
- Global Socket Statistics (will only work if the url is `http://127.0.0.1:8888/json/v1`, i.e. without `/server`, but keep in mind this produces a very long output and probably will account for 0.5% CPU overhead alone, per bind server added)
- Per View Statistics (the following set will be added for each bind view):
   - View, Resolver Active Queries
   - View, Resolver Statistics
   - View, Resolver Round Trip Timings
   - View, Requests by Query Type

## Configuration

The collector (optionally) reads a configuration file named `/etc/netdata/node.d/named.conf`, with the following contents:

```js
{
    "enable_autodetect": true,
    "update_every": 5,
    "servers": [
        {
            "name": "bind1",
            "url": "http://127.0.0.1:8888/json/v1/server",
            "update_every": 1
        },
        {
            "name": "bind2",
            "url": "http://10.1.2.3:8888/json/v1/server",
            "update_every": 2
        }
    ]
}
```

You can add any number of bind servers.

If the configuration file is missing, or the key `enable_autodetect` is `true`, the collector will also attempt to fetch `http://localhost:8888/json/v1/server` which, if successful will be added too.

### XML instead of JSON, from bind

The collector can also accept bind URLs that return XML output. This might required if you cannot have bind 9.10+ with JSON but you have an version of bind that supports XML statistics v3. Check [this](https://www.isc.org/blogs/bind-9-10-statistics-troubleshooting-and-zone-configuration/) for versions supported.

In such cases, use a URL like this:

```sh
curl "http://localhost:8888/xml/v3/server"
```

Only `xml` and `v3` has been tested.

Keep in mind though, that XML parsing is done using javascript code, which requires a triple conversion:

1. from XML to JSON using a javascript XML parser (**CPU intensive**),
2. which is then transformed to emulate the output of the JSON output of bind (**CPU intensive** - and yes the converted JSON from XML is different to the native JSON - even bind produces different names for various attributes),
3. which is then processed to generate the data for the charts (this will happen even if bind is producing JSON).

In general, expect XML parsing to be 2 to 3 times more CPU intensive than JSON.

**So, if you can use the JSON output of bind, prefer it over XML**. Keep also in mind that even bind will use more CPU when generating XML instead of JSON.

The XML interface of bind is not autodetected.
You will have to provide the config file `/etc/netdata/node.d/named.conf`, like this:

```js
{
    "enable_autodetect": false,
    "update_every": 1,
    "servers": [
        {
            "name": "local",
            "url": "http://localhost:8888/xml/v3/server",
            "update_every": 1
        }
    ]
}
```

Of course, you can monitor more than one bind servers. Each one can be configured with either JSON or XML output.

## Auto-detection

Auto-detection is controlled by `enable_autodetect` in the config file. The default is enabled, so that if the collector can connect to `http://localhost:8888/json/v1/server` to receive bind statistics, it will automatically enable it.

## Bind (named) configuration

To use this plugin, you have to have bind v9.10+ properly compiled to provide statistics in `JSON` format.

For more information on how to get your bind installation ready, please refer to the [bind statistics channel developer comments](http://jpmens.net/2013/03/18/json-in-bind-9-s-statistics-server/) and to [bind documentation](https://ftp.isc.org/isc/bind/9.10.3/doc/arm/Bv9ARM.ch06.html#statistics) or [bind Knowledge Base article AA-01123](https://kb.isc.org/article/AA-01123/0).

Normally, you will need something like this in your `named.conf`:

```
statistics-channels {
        inet 127.0.0.1 port 8888 allow { 127.0.0.1; };
        inet ::1 port 8888 allow { ::1; };
};
```

(use the IPv4 or IPv6 line depending on what you are using, you can also use both)

Verify it works by running the following command (the collector is written in node.js and will query your bind server directly, but if this command works, the collector should be able to work too):

```sh
curl "http://localhost:8888/json/v1/server"
```

