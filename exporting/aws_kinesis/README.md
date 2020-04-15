<!--
---
title: "Export metrics to AWS Kinesis Data Streams"
custom_edit_url: https://github.com/netdata/netdata/edit/master/exporting/aws_kinesis/README.md
---
-->

# Export metrics to AWS Kinesis Data Streams

## Prerequisites

To use AWS Kinesis for metric collecting and processing, you should first
[install](https://docs.aws.amazon.com/en_us/sdk-for-cpp/v1/developer-guide/setup.html) AWS SDK for C++. Netdata
works with the SDK version 1.7.121. Other versions might work correctly as well, but they were not tested with Netdata.
`libcrypto`, `libssl`, and `libcurl` are also required to compile Netdata with Kinesis support enabled. Next, Netdata
should be re-installed from the source. The installer will detect that the required libraries are now available.

If the AWS SDK for C++ is being installed from source, it is useful to set `-DBUILD_ONLY="kinesis"`. Otherwise, the
building process could take a very long time. Note that the default installation path for the libraries is
`/usr/local/lib64`. Many Linux distributions don't include this path as the default one for a library search, so it is
advisable to use the following options to `cmake` while building the AWS SDK:

```sh
cmake -DCMAKE_INSTALL_LIBDIR=/usr/lib -DCMAKE_INSTALL_INCLUDEDIR=/usr/include -DBUILD_SHARED_LIBS=OFF -DBUILD_ONLY=kinesis <aws-sdk-cpp sources>
```

## Configuration

To enable data sending to the Kinesis service, run `./edit-config exporting.conf` in the Netdata configuration directory
and set the following options:

```conf
[kinesis:my_instance]
    enabled = yes
    destination = us-east-1
```

Set the `destination` option to an AWS region.

Set AWS credentials and stream name:

```conf
    # AWS credentials
    aws_access_key_id = your_access_key_id
    aws_secret_access_key = your_secret_access_key
    # destination stream
    stream name = your_stream_name
```

Alternatively, you can set AWS credentials for the `netdata` user using AWS SDK for C++ [standard methods](https://docs.aws.amazon.com/sdk-for-cpp/v1/developer-guide/credentials.html).

Netdata automatically computes a partition key for every record with the purpose to distribute records across
available shards evenly.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fexporting%2Faws_kinesis%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
