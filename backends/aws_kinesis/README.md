# Using Netdata with AWS Kinesis Data Streams

## Prerequisites

To use AWS Kinesis as a backend AWS SDK for C++ should be [installed](https://docs.aws.amazon.com/en_us/sdk-for-cpp/v1/developer-guide/setup.html) first. `libcrypto`, `libssl`, and `libcurl` are also required to compile Netdata with Kinesis support enabled. Next, Netdata should be re-installed from the source. The installer will detect that the required libraries are now available.

If the AWS SDK for C++ is being installed from source, it is useful to set `-DBUILD_ONLY="kinesis"`. Otherwise, the building process could take a very long time. Take a note, that the default installation path for the libraries is `/usr/local/lib64`. Many Linux distributions don't include this path as the default one for a library search, so it is advisable to use the following options to `cmake` while building the AWS SDK:

```
cmake -DCMAKE_INSTALL_LIBDIR=/usr/lib -DCMAKE_INSTALL_INCLUDEDIR=/usr/include -DBUILD_SHARED_LIBS=OFF -DBUILD_ONLY=kinesis <aws-sdk-cpp sources>
```

## Configuration

To enable data sending to the kinesis backend set the following options in `netdata.conf`:
```
[backend]
    enabled = yes
    type = kinesis
    destination = us-east-1
```
set the `destination` option to an AWS region.

In the Netdata configuration directory run `./edit-config aws_kinesis.conf` and set AWS credentials and stream name:
```
# AWS credentials
aws_access_key_id = your_access_key_id
aws_secret_access_key = your_secret_access_key

# destination stream
stream name = your_stream_name
```
Alternatively, AWS credentials can be set for the *netdata* user using AWS SDK for C++ [standard methods](https://docs.aws.amazon.com/sdk-for-cpp/v1/developer-guide/credentials.html).

A partition key for every record is computed automatically by Netdata with the purpose to distribute records across available shards evenly.


[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fbackends%2Faws_kinesis%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
