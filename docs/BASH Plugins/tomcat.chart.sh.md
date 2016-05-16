Plugin contributed by [jgeromero](https://github.com/jgeromero) with PR [#277](https://github.com/firehol/netdata/pull/277).

* Tested on Tomcat Version 7.0.54 on Ubuntu Server 14.04.1 LTS
* Requires xmlstarlet

Screenshot:

![tomcat-netdata](https://cloud.githubusercontent.com/assets/9483354/14687417/5ee7cc66-070b-11e6-8483-17c3a8e0c871.jpg)

The plugin can monitor only one tomcat server. The default URL it expects is: `http://localhost:8080/manager/status?XML=true`.

The BASH source code of the plugin is here: https://github.com/firehol/netdata/blob/master/charts.d/tomcat.chart.sh