
// Registry of netdata hosts

NETDATA.registry = {
    server: null,         // the netdata registry server
    cloudBaseURL: null,   // the netdata cloud base url
    person_guid: null,    // the unique ID of this browser / user
    machine_guid: null,   // the unique ID the netdata server that served dashboard.js
    hostname: 'unknown',  // the hostname of the netdata server that served dashboard.js
    machines: null,       // the user's other URLs
    machines_array: null, // the user's other URLs in an array
    person_urls: null,

    parsePersonUrls: function (person_urls) {
        // console.log(person_urls);
        NETDATA.registry.person_urls = person_urls;

        if (person_urls) {
            NETDATA.registry.machines = {};
            NETDATA.registry.machines_array = [];

            let apu = person_urls;
            let i = apu.length;
            while (i--) {
                if (typeof NETDATA.registry.machines[apu[i][0]] === 'undefined') {
                    // console.log('adding: ' + apu[i][4] + ', ' + ((now - apu[i][2]) / 1000).toString());

                    let obj = {
                        guid: apu[i][0],
                        url: apu[i][1],
                        last_t: apu[i][2],
                        accesses: apu[i][3],
                        name: apu[i][4],
                        alternate_urls: []
                    };
                    obj.alternate_urls.push(apu[i][1]);

                    NETDATA.registry.machines[apu[i][0]] = obj;
                    NETDATA.registry.machines_array.push(obj);
                } else {
                    // console.log('appending: ' + apu[i][4] + ', ' + ((now - apu[i][2]) / 1000).toString());

                    let pu = NETDATA.registry.machines[apu[i][0]];
                    if (pu.last_t < apu[i][2]) {
                        pu.url = apu[i][1];
                        pu.last_t = apu[i][2];
                        pu.name = apu[i][4];
                    }
                    pu.accesses += apu[i][3];
                    pu.alternate_urls.push(apu[i][1]);
                }
            }
        }

        if (typeof netdataRegistryCallback === 'function') {
            netdataRegistryCallback(NETDATA.registry.machines_array);
        }
    },

    init: function () {
        if (netdataRegistry !== true) {
            return;
        }

        NETDATA.registry.hello(NETDATA.serverDefault, function (data) {
            if (data) {
                NETDATA.registry.server = data.registry;
                NETDATA.registry.cloudBaseURL = data.cloud_base_url;
                NETDATA.registry.machine_guid = data.machine_guid;
                NETDATA.registry.hostname = data.hostname;

                NETDATA.registry.access(2, function (person_urls) {
                    NETDATA.registry.parsePersonUrls(person_urls);

                });
            }
        });
    },

    hello: function (host, callback) {
        host = NETDATA.fixHost(host);

        // send HELLO to a netdata server:
        // 1. verifies the server is reachable
        // 2. responds with the registry URL, the machine GUID of this netdata server and its hostname
        $.ajax({
            url: host + '/api/v1/registry?action=hello',
            async: true,
            cache: false,
            headers: {
                'Cache-Control': 'no-cache, no-store',
                'Pragma': 'no-cache'
            },
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function (data) {
                data = NETDATA.xss.checkOptional('/api/v1/registry?action=hello', data);

                if (typeof data.status !== 'string' || data.status !== 'ok') {
                    NETDATA.error(408, host + ' response: ' + JSON.stringify(data));
                    data = null;
                }

                if (typeof callback === 'function') {
                    return callback(data);
                }
            })
            .fail(function () {
                NETDATA.error(407, host);

                if (typeof callback === 'function') {
                    return callback(null);
                }
            });
    },

    access: function (max_redirects, callback) {
        // send ACCESS to a netdata registry:
        // 1. it lets it know we are accessing a netdata server (its machine GUID and its URL)
        // 2. it responds with a list of netdata servers we know
        // the registry identifies us using a cookie it sets the first time we access it
        // the registry may respond with a redirect URL to send us to another registry
        $.ajax({
            url: NETDATA.registry.server + '/api/v1/registry?action=access&machine=' + NETDATA.registry.machine_guid + '&name=' + encodeURIComponent(NETDATA.registry.hostname) + '&url=' + encodeURIComponent(NETDATA.serverDefault), // + '&visible_url=' + encodeURIComponent(document.location),
            async: true,
            cache: false,
            headers: {
                'Cache-Control': 'no-cache, no-store',
                'Pragma': 'no-cache'
            },
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function (data) {
                data = NETDATA.xss.checkAlways('/api/v1/registry?action=access', data);

                let redirect = null;
                if (typeof data.registry === 'string') {
                    redirect = data.registry;
                }

                if (typeof data.status !== 'string' || data.status !== 'ok') {
                    NETDATA.error(409, NETDATA.registry.server + ' responded with: ' + JSON.stringify(data));
                    data = null;
                }

                if (data === null) {
                    if (redirect !== null && max_redirects > 0) {
                        NETDATA.registry.server = redirect;
                        NETDATA.registry.access(max_redirects - 1, callback);
                    }
                    else {
                        if (typeof callback === 'function') {
                            return callback(null);
                        }
                    }
                }
                else {
                    if (typeof data.person_guid === 'string') {
                        NETDATA.registry.person_guid = data.person_guid;
                    }

                    if (typeof callback === 'function') {
                        return callback(data.urls);
                    }
                }
            })
            .fail(function () {
                NETDATA.error(410, NETDATA.registry.server);

                if (typeof callback === 'function') {
                    return callback(null);
                }
            });
    },

    delete: function (delete_url, callback) {
        // send DELETE to a netdata registry:
        $.ajax({
            url: NETDATA.registry.server + '/api/v1/registry?action=delete&machine=' + NETDATA.registry.machine_guid + '&name=' + encodeURIComponent(NETDATA.registry.hostname) + '&url=' + encodeURIComponent(NETDATA.serverDefault) + '&delete_url=' + encodeURIComponent(delete_url),
            async: true,
            cache: false,
            headers: {
                'Cache-Control': 'no-cache, no-store',
                'Pragma': 'no-cache'
            },
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function (data) {
                data = NETDATA.xss.checkAlways('/api/v1/registry?action=delete', data);

                if (typeof data.status !== 'string' || data.status !== 'ok') {
                    NETDATA.error(411, NETDATA.registry.server + ' responded with: ' + JSON.stringify(data));
                    data = null;
                }

                if (typeof callback === 'function') {
                    return callback(data);
                }
            })
            .fail(function () {
                NETDATA.error(412, NETDATA.registry.server);

                if (typeof callback === 'function') {
                    return callback(null);
                }
            });
    },

    search: function (machine_guid, callback) {
        // SEARCH for the URLs of a machine:
        $.ajax({
            url: NETDATA.registry.server + '/api/v1/registry?action=search&machine=' + NETDATA.registry.machine_guid + '&name=' + encodeURIComponent(NETDATA.registry.hostname) + '&url=' + encodeURIComponent(NETDATA.serverDefault) + '&for=' + machine_guid,
            async: true,
            cache: false,
            headers: {
                'Cache-Control': 'no-cache, no-store',
                'Pragma': 'no-cache'
            },
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function (data) {
                data = NETDATA.xss.checkAlways('/api/v1/registry?action=search', data);

                if (typeof data.status !== 'string' || data.status !== 'ok') {
                    NETDATA.error(417, NETDATA.registry.server + ' responded with: ' + JSON.stringify(data));
                    data = null;
                }

                if (typeof callback === 'function') {
                    return callback(data);
                }
            })
            .fail(function () {
                NETDATA.error(418, NETDATA.registry.server);

                if (typeof callback === 'function') {
                    return callback(null);
                }
            });
    },

    switch: function (new_person_guid, callback) {
        // impersonate
        $.ajax({
            url: NETDATA.registry.server + '/api/v1/registry?action=switch&machine=' + NETDATA.registry.machine_guid + '&name=' + encodeURIComponent(NETDATA.registry.hostname) + '&url=' + encodeURIComponent(NETDATA.serverDefault) + '&to=' + new_person_guid,
            async: true,
            cache: false,
            headers: {
                'Cache-Control': 'no-cache, no-store',
                'Pragma': 'no-cache'
            },
            xhrFields: {withCredentials: true} // required for the cookie
        })
            .done(function (data) {
                data = NETDATA.xss.checkAlways('/api/v1/registry?action=switch', data);

                if (typeof data.status !== 'string' || data.status !== 'ok') {
                    NETDATA.error(413, NETDATA.registry.server + ' responded with: ' + JSON.stringify(data));
                    data = null;
                }

                if (typeof callback === 'function') {
                    return callback(data);
                }
            })
            .fail(function () {
                NETDATA.error(414, NETDATA.registry.server);

                if (typeof callback === 'function') {
                    return callback(null);
                }
            });
    }
};
