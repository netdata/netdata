# Java Plugin

[![Build Status](https://travis-ci.org/simonnagl/netdata-plugin-java-daemon.svg?branch=master)](https://travis-ci.org/simonnagl/netdata-plugin-java-daemon)
[![Codacy Badge](https://api.codacy.com/project/badge/Coverage/c5196ea860ba4cb8a47f40c5264cc17f)](https://www.codacy.com/app/simonnagl/netdata-plugin-java-daemon?utm_source=github.com&utm_medium=referral&utm_content=simonnagl/netdata-plugin-java-daemon&utm_campaign=Badge_Coverage)
[![Codacy Badge](https://api.codacy.com/project/badge/Grade/c5196ea860ba4cb8a47f40c5264cc17f)](https://www.codacy.com/app/simonnagl/netdata-plugin-java-daemon?utm_source=github.com&utm_medium=referral&utm_content=simonnagl/netdata-plugin-java-daemon&utm_campaign=badger)
[![Dependency Status](https://www.versioneye.com/user/projects/59994481368b08135edcaabe/badge.svg?style=flat-square)](https://www.versioneye.com/user/projects/59994481368b08135edcaabe)

## Modules

- JMX Collector

## Required for compilation

- JDK 8.x

## Configuration

Configuration files contain JSON Objects.
Additional to the JSON specification Java/C++ style comments (both '/'+'*' and '//' varieties) are allowed.

Each plugin get's it's own configuration file. The standard configuration should have enogh examples and comments to extend or adapt it. The table below references the classes which describe the JSON schemes of the configuration files.

File                         | Schema | Purpose
---------------------------- | ------ | -------
/etc/netdata/java.d/jmx.conf | [JmxPluginConfiguration](https://github.com/firehol/netdata/blob/master/java.d/src/main/java/org/firehol/netdata/plugin/jmx/configuration/JmxPluginConfiguration.java)| JMX plugin configuration
