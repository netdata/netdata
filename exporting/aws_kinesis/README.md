<!--
title: "Export metrics to AWS Kinesis Data Streams"
description: "Archive your Agent's metrics to AWS Kinesis Data Streams for long-term storage, further analysis, or correlation with data from other sources."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/exporting/aws_kinesis/README.md"
sidebar_label: "AWS Kinesis Data Streams"
learn_status: "Published"
learn_rel_path: "Integrations/Export"
-->

# Export metrics to AWS Kinesis Data Streams

## Prerequisites

To use AWS Kinesis for metric collecting and processing, you should first
[install](https://docs.aws.amazon.com/en_us/sdk-for-cpp/v1/developer-guide/setup.html) AWS SDK for C++.
`libcrypto`, `libssl`, and `libcurl` are also required to compile Netdata with Kinesis support enabled. Next, Netdata
should be re-installed from the source. The installer will detect that the required libraries are now available.

If the AWS SDK for C++ is being installed from source, it is useful to set `-DBUILD_ONLY=kinesis`. Otherwise, the
build process could take a very long time. Note, that the default installation path for the libraries is
`/usr/local/lib64`. Many Linux distributions don't include this path as the default one for a library search, so it is
advisable to use the following options to `cmake` while building the AWS SDK:

```sh
sudo cmake -DCMAKE_INSTALL_PREFIX=/usr -DBUILD_ONLY=kinesis <aws-sdk-cpp sources>
```

The `-DCMAKE_INSTALL_PREFIX=/usr` option also ensures that
[third party dependencies](https://github.com/aws/aws-sdk-cpp#third-party-dependencies) are installed in your system
during the SDK build process.

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

Alternatively, you can set AWS credentials for the `netdata` user using AWS SDK for
C++ [standard methods](https://docs.aws.amazon.com/sdk-for-cpp/v1/developer-guide/credentials.html).

Netdata automatically computes a partition key for every record with the purpose to distribute records across
available shards evenly.


