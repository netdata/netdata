
// *** src/dashboard.js/chart-registry.js

// Chart Registry

// When multiple charts need the same chart, we avoid downloading it
// multiple times (and having it in browser memory multiple time)
// by using this registry.

// Every time we download a chart definition, we save it here with .add()
// Then we try to get it back with .get(). If that fails, we download it.

NETDATA.fixHost = function (host) {
    while (host.slice(-1) === '/') {
        host = host.substring(0, host.length - 1);
    }

    return host;
};

NETDATA.chartRegistry = {
    charts: {},

    globalReset: function () {
        this.charts = {};
    },

    add: function (host, id, data) {
        if (typeof this.charts[host] === 'undefined') {
            this.charts[host] = {};
        }

        //console.log('added ' + host + '/' + id);
        this.charts[host][id] = data;
    },

    get: function (host, id) {
        if (typeof this.charts[host] === 'undefined') {
            return null;
        }

        if (typeof this.charts[host][id] === 'undefined') {
            return null;
        }

        //console.log('cached ' + host + '/' + id);
        return this.charts[host][id];
    },

    downloadAll: function (host, callback) {
        host = NETDATA.fixHost(host);

        let self = this;

        function got_data(h, data, callback) {
            if (data !== null) {
                self.charts[h] = data.charts;

                // update the server timezone in our options
                if (typeof data.timezone === 'string') {
                    NETDATA.options.server_timezone = data.timezone;
                }
            } else {
                NETDATA.error(406, h + '/api/v1/charts');
            }

            if (typeof callback === 'function') {
                callback(data);
            }
        }

        if (netdataSnapshotData !== null) {
            got_data(host, netdataSnapshotData.charts, callback);
        } else {
            $.ajax({
                url: host + '/api/v1/charts',
                async: true,
                cache: false,
                xhrFields: {withCredentials: true} // required for the cookie
            })
                .done(function (data) {
                    data = NETDATA.xss.checkOptional('/api/v1/charts', data);
                    got_data(host, data, callback);
                })
                .fail(function () {
                    NETDATA.error(405, host + '/api/v1/charts');

                    if (typeof callback === 'function') {
                        callback(null);
                    }
                });
        }
    }
};
