Plugin contributed by [safeie](https://github.com/safeie) with PR [#276](https://github.com/firehol/netdata/pull/276).

PHP-FPM (FastCGI Process Manager) is an alternative PHP FastCGI implementation with some additional features useful for sites of any size, especially busier sites.

Information about configuring your nginx to expose php-fpm status information: https://easyengine.io/tutorials/php/fpm-status-page/

The plugin, by default expects php-fpm information on this url: `http://localhost/status`.

The BASH source code of the plugin is here: https://github.com/firehol/netdata/blob/master/charts.d/phpfpm.chart.sh

The plugin can monitor multiple php-fpm servers.