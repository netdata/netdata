# Java Plugin

## Requirements

- JDK 8.x


## Modules

The following python.d modules are supported:

### JMX

This module will monitor all local java processes by default. Java Monitoring Extension (JMX) is used to do so.

It produces the following charts per process if the JVM supports them:

- **CPU** usage in percent
- **Load** of the last minute
- **Uptime** of the process
- **Threading**

## Configuration

Configuration files contain JSON Objects.
Additional to the JSON specification Java/C++ style comments (both '/'+'*' and '//' varieties) are allowed.

Each plugin get's it's own configuration file. The standard configuration should have enogh examples and comments to extend or adapt it. The table below references the classes which describe the JSON schemes of the configuration files.

File                         | Schema | Purpose
---------------------------- | ------ | -------
/etc/netdata/java.d/jmx.conf | [JmxPluginConfiguration](https://github.com/firehol/netdata/blob/master/java.d/src/main/java/org/firehol/netdata/plugin/jmx/configuration/JmxPluginConfiguration.java)| JMX plugin configuration
