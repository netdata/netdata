# Java Plugin

## Requirements

- JDK 8.x


## Modules

The following `java.d` modules are supported:

### JMX

This module will monitor all local java processes by default. Java Monitoring Extension (JMX) is used to do so.

## Configuration

Configuration files contain JSON Objects.
Additional to the JSON specification Java/C++ style comments (both '/'+'*' and '//' varieties) are allowed.

Each plugin get's it's own configuration file. The standard configuration should have enogh examples and comments to extend or adapt it. The table below references the classes which describe the JSON schemes of the configuration files.

File                         | Schema | Purpose
---------------------------- | ------ | -------
/etc/netdata/java.d/jmx.conf | [JmxPluginConfiguration](https://github.com/firehol/netdata/blob/master/java.d/src/main/java/org/firehol/netdata/plugin/jmx/configuration/JmxPluginConfiguration.java)| JMX plugin configuration

## How to Write New Module

This section is about writing a new module for the `java.d` plugin, for other uses see [How to Write New Module (`python.d`)](https://github.com/firehol/netdata/wiki/How-to-write-new-module) or [Monitoring Java Spring Boot Applications](https://github.com/firehol/netdata/wiki/Monitoring-Java-Spring-Boot-Applications).

Writing a new Java module is done as follows.

- Implement a subclass of [Module](https://github.com/firehol/netdata/blob/master/java.d/src/main/java/org/firehol/netdata/plugin/Module.java)
  * `initialize()` to optionally read configuration and optionally provide a list of charts
  * `collectValues()` will be called periodically (`updateEvery`) and should return the list of charts with dimensions having their values set (if known)
  * a subclass of `Module.Builder` which knows how to call your module class' constructor
- Register your module in `java.d.conf` under the `modules` section as `"your_module": "your.package.YourModule$Builder"`
- If your module should be configured by a file:
  * create the configuration file `$prefix/etc/netdata/java.d/your_module.conf` (JSON with Javascript-style comments)
  * implement the matching JSON deserializable configuration scheme, e.g. `your.package.configuration.YourModuleConfiguration`
  * in `initialize()`, call `configurationService.getModuleConfiguration(getName(), YourModuleConfiguration.class)` to access the configuration

For more details, see the Javadoc of [Module](https://github.com/firehol/netdata/blob/master/java.d/src/main/java/org/firehol/netdata/plugin/Module.java) or the source code of [ExampleModule](https://github.com/firehol/netdata/blob/master/java.d/src/main/java/org/firehol/netdata/plugin/example/ExampleModule.java).