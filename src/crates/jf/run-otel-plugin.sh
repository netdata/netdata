#!/bin/bash

# Script to run otel-plugin standalone without netdata server
# This sets up the required environment variables for testing


set -exu -o pipefail

just ninja install
clear

env NETDATA_STOCK_CONFIG_DIR="/Users/vk/opt/otel-plugin/netdata/usr/lib/netdata/conf.d" \
    NETDATA_USER_CONFIG_DIR="/Users/vk/opt/otel-plugin/netdata/etc/netdata" \
    NETDATA_PLUGINS_DIR="/Users/vk/opt/otel-plugin/netdata/usr/libexec/netdata/plugins.d" \
    NETDATA_LOG_DIR="/Users/vk/opt/otel-plugin/netdata/var/log/netdata" \
    NETDATA_INVOCATION_ID="test-123" \
    /Users/vk/opt/otel-plugin/netdata/usr/libexec/netdata/plugins.d/otel-plugin "$@"