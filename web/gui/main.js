// Main JavaScript file for the Netdata GUI.

// netdata snapshot data
var netdataSnapshotData = null;

// enable alarms checking and notifications
var netdataShowAlarms = true;

// enable registry updates
var netdataRegistry = true;

// forward definition only - not used here
var netdataServer = undefined;
var netdataServerStatic = undefined;
var netdataCheckXSS = undefined;

// control the welcome modal and analytics
var this_is_demo = null;

function escapeUserInputHTML(s) {
    return s.toString()
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/#/g, '&#35;')
        .replace(/'/g, '&#39;')
        .replace(/\(/g, '&#40;')
        .replace(/\)/g, '&#41;')
        .replace(/\//g, '&#47;');
}

function verifyURL(s) {
    if (typeof (s) === 'string' && (s.startsWith('http://') || s.startsWith('https://'))) {
        return s
            .replace(/'/g, '%22')
            .replace(/"/g, '%27')
            .replace(/\)/g, '%28')
            .replace(/\(/g, '%29');
    }

    console.log('invalid URL detected:');
    console.log(s);
    return 'javascript:alert("invalid url");';
}

// --------------------------------------------------------------------
// urlOptions

var urlOptions = {
    hash: '#',
    theme: null,
    help: null,
    mode: 'live',         // 'live', 'print'
    update_always: false,
    pan_and_zoom: false,
    server: null,
    after: 0,
    before: 0,
    highlight: false,
    highlight_after: 0,
    highlight_before: 0,
    nowelcome: false,
    show_alarms: false,
    chart: null,
    family: null,
    alarm: null,
    alarm_unique_id: 0,
    alarm_id: 0,
    alarm_event_id: 0,

    hasProperty: function (property) {
        // console.log('checking property ' + property + ' of type ' + typeof(this[property]));
        return typeof this[property] !== 'undefined';
    },

    genHash: function (forReload) {
        var hash = urlOptions.hash;

        if (urlOptions.pan_and_zoom === true) {
            hash += ';after=' + urlOptions.after.toString() +
                ';before=' + urlOptions.before.toString();
        }

        if (urlOptions.highlight === true) {
            hash += ';highlight_after=' + urlOptions.highlight_after.toString() +
                ';highlight_before=' + urlOptions.highlight_before.toString();
        }

        if (urlOptions.theme !== null) {
            hash += ';theme=' + urlOptions.theme.toString();
        }

        if (urlOptions.help !== null) {
            hash += ';help=' + urlOptions.help.toString();
        }

        if (urlOptions.update_always === true) {
            hash += ';update_always=true';
        }

        if (forReload === true && urlOptions.server !== null) {
            hash += ';server=' + urlOptions.server.toString();
        }

        if (urlOptions.mode !== 'live') {
            hash += ';mode=' + urlOptions.mode;
        }

        return hash;
    },

    parseHash: function () {
        var variables = document.location.hash.split(';');
        var len = variables.length;
        while (len--) {
            if (len !== 0) {
                var p = variables[len].split('=');
                if (urlOptions.hasProperty(p[0]) && typeof p[1] !== 'undefined') {
                    urlOptions[p[0]] = decodeURIComponent(p[1]);
                }
            } else {
                if (variables[len].length > 0) {
                    urlOptions.hash = variables[len];
                }
            }
        }

        var booleans = ['nowelcome', 'show_alarms', 'update_always'];
        len = booleans.length;
        while (len--) {
            if (urlOptions[booleans[len]] === 'true' || urlOptions[booleans[len]] === true || urlOptions[booleans[len]] === '1' || urlOptions[booleans[len]] === 1) {
                urlOptions[booleans[len]] = true;
            } else {
                urlOptions[booleans[len]] = false;
            }
        }

        var numeric = ['after', 'before', 'highlight_after', 'highlight_before'];
        len = numeric.length;
        while (len--) {
            if (typeof urlOptions[numeric[len]] === 'string') {
                try {
                    urlOptions[numeric[len]] = parseInt(urlOptions[numeric[len]]);
                }
                catch (e) {
                    console.log('failed to parse URL hash parameter ' + numeric[len]);
                    urlOptions[numeric[len]] = 0;
                }
            }
        }

        if (urlOptions.server !== null && urlOptions.server !== '') {
            netdataServerStatic = document.location.origin.toString() + document.location.pathname.toString();
            netdataServer = urlOptions.server;
            netdataCheckXSS = true;
        } else {
            urlOptions.server = null;
        }

        if (urlOptions.before > 0 && urlOptions.after > 0) {
            urlOptions.pan_and_zoom = true;
            urlOptions.nowelcome = true;
        } else {
            urlOptions.pan_and_zoom = false;
        }

        if (urlOptions.highlight_before > 0 && urlOptions.highlight_after > 0) {
            urlOptions.highlight = true;
        } else {
            urlOptions.highlight = false;
        }

        switch (urlOptions.mode) {
            case 'print':
                urlOptions.theme = 'white';
                urlOptions.welcome = false;
                urlOptions.help = false;
                urlOptions.show_alarms = false;

                if (urlOptions.pan_and_zoom === false) {
                    urlOptions.pan_and_zoom = true;
                    urlOptions.before = Date.now();
                    urlOptions.after = urlOptions.before - 600000;
                }

                netdataShowAlarms = false;
                netdataRegistry = false;
                this_is_demo = false;
                break;

            case 'live':
            default:
                urlOptions.mode = 'live';
                break;
        }

        // console.log(urlOptions);
    },

    hashUpdate: function () {
        history.replaceState(null, '', urlOptions.genHash(true));
    },

    netdataPanAndZoomCallback: function (status, after, before) {
        //console.log(1);
        //console.log(new Error().stack);

        if (netdataSnapshotData === null) {
            urlOptions.pan_and_zoom = status;
            urlOptions.after = after;
            urlOptions.before = before;
            urlOptions.hashUpdate();
        }
    },

    netdataHighlightCallback: function (status, after, before) {
        //console.log(2);
        //console.log(new Error().stack);

        if (status === true && (after === null || before === null || after <= 0 || before <= 0 || after >= before)) {
            status = false;
            after = 0;
            before = 0;
        }

        if (netdataSnapshotData === null) {
            urlOptions.highlight = status;
        } else {
            urlOptions.highlight = false;
        }

        urlOptions.highlight_after = Math.round(after);
        urlOptions.highlight_before = Math.round(before);
        urlOptions.hashUpdate();

        var show_eye = NETDATA.globalChartUnderlay.hasViewport();

        if (status === true && after > 0 && before > 0 && after < before) {
            var d1 = NETDATA.dateTime.localeDateString(after);
            var d2 = NETDATA.dateTime.localeDateString(before);
            if (d1 === d2) {
                d2 = '';
            }
            document.getElementById('navbar-highlight-content').innerHTML =
                ((show_eye === true) ? '<span class="navbar-highlight-bar highlight-tooltip" onclick="urlOptions.showHighlight();" title="restore the highlighted view" data-toggle="tooltip" data-placement="bottom">' : '<span>').toString()
                + 'highlighted time-frame'
                + ' <b>' + d1 + ' <code>' + NETDATA.dateTime.localeTimeString(after) + '</code></b> to '
                + ' <b>' + d2 + ' <code>' + NETDATA.dateTime.localeTimeString(before) + '</code></b>, '
                + 'duration <b>' + NETDATA.seconds4human(Math.round((before - after) / 1000)) + '</b>'
                + '</span>'
                + '<span class="navbar-highlight-button-right highlight-tooltip" onclick="urlOptions.clearHighlight();" title="clear the highlighted time-frame" data-toggle="tooltip" data-placement="bottom"><i class="fas fa-times"></i></span>';

            $('.navbar-highlight').show();

            $('.highlight-tooltip').tooltip({
                html: true,
                delay: {show: 500, hide: 0},
                container: 'body'
            });
        } else {
            $('.navbar-highlight').hide();
        }
    },

    clearHighlight: function () {
        NETDATA.globalChartUnderlay.clear();

        if (NETDATA.globalPanAndZoom.isActive() === true) {
            NETDATA.globalPanAndZoom.clearMaster();
        }
    },

    showHighlight: function () {
        NETDATA.globalChartUnderlay.focus();
    }
};

urlOptions.parseHash();

// --------------------------------------------------------------------
// check options that should be processed before loading netdata.js

var localStorageTested = -1;

function localStorageTest() {
    if (localStorageTested !== -1) {
        return localStorageTested;
    }

    if (typeof Storage !== "undefined" && typeof localStorage === 'object') {
        var test = 'test';
        try {
            localStorage.setItem(test, test);
            localStorage.removeItem(test);
            localStorageTested = true;
        }
        catch (e) {
            console.log(e);
            localStorageTested = false;
        }
    } else {
        localStorageTested = false;
    }

    return localStorageTested;
}

function loadLocalStorage(name) {
    var ret = null;

    try {
        if (localStorageTest() === true) {
            ret = localStorage.getItem(name);
        } else {
            console.log('localStorage is not available');
        }
    }
    catch (error) {
        console.log(error);
        return null;
    }

    if (typeof ret === 'undefined' || ret === null) {
        return null;
    }

    // console.log('loaded: ' + name.toString() + ' = ' + ret.toString());

    return ret;
}

function saveLocalStorage(name, value) {
    // console.log('saving: ' + name.toString() + ' = ' + value.toString());
    try {
        if (localStorageTest() === true) {
            localStorage.setItem(name, value.toString());
            return true;
        }
    }
    catch (error) {
        console.log(error);
    }

    return false;
}

function getTheme(def) {
    if (urlOptions.mode === 'print') {
        return 'white';
    }

    var ret = loadLocalStorage('netdataTheme');
    if (typeof ret === 'undefined' || ret === null || ret === 'undefined') {
        return def;
    } else {
        return ret;
    }
}

function setTheme(theme) {
    if (urlOptions.mode === 'print') {
        return false;
    }

    if (theme === netdataTheme) {
        return false;
    }
    return saveLocalStorage('netdataTheme', theme);
}

var netdataTheme = getTheme('slate');
var netdataShowHelp = true;

if (urlOptions.theme !== null) {
    setTheme(urlOptions.theme);
    netdataTheme = urlOptions.theme;
} else {
    urlOptions.theme = netdataTheme;
}

if (urlOptions.help !== null) {
    saveLocalStorage('options.show_help', urlOptions.help);
    netdataShowHelp = urlOptions.help;
} else {
    urlOptions.help = loadLocalStorage('options.show_help');
}

// --------------------------------------------------------------------
// natural sorting
// http://www.davekoelle.com/files/alphanum.js - LGPL

function naturalSortChunkify(t) {
    var tz = [];
    var x = 0, y = -1, n = 0, i, j;

    while (i = (j = t.charAt(x++)).charCodeAt(0)) {
        var m = (i >= 48 && i <= 57);
        if (m !== n) {
            tz[++y] = "";
            n = m;
        }
        tz[y] += j;
    }

    return tz;
}

function naturalSortCompare(a, b) {
    var aa = naturalSortChunkify(a.toLowerCase());
    var bb = naturalSortChunkify(b.toLowerCase());

    for (var x = 0; aa[x] && bb[x]; x++) {
        if (aa[x] !== bb[x]) {
            var c = Number(aa[x]), d = Number(bb[x]);
            if (c.toString() === aa[x] && d.toString() === bb[x]) {
                return c - d;
            } else {
                return (aa[x] > bb[x]) ? 1 : -1;
            }
        }
    }

    return aa.length - bb.length;
}

// --------------------------------------------------------------------
// saving files to client

function saveTextToClient(data, filename) {
    var blob = new Blob([data], {
        type: 'application/octet-stream'
    });

    var url = URL.createObjectURL(blob);
    var link = document.createElement('a');
    link.setAttribute('href', url);
    link.setAttribute('download', filename);

    var el = document.getElementById('hiddenDownloadLinks');
    el.innerHTML = '';
    el.appendChild(link);

    setTimeout(function () {
        el.removeChild(link);
        URL.revokeObjectURL(url);
    }, 60);

    link.click();
}

function saveObjectToClient(data, filename) {
    saveTextToClient(JSON.stringify(data), filename);
}

// --------------------------------------------------------------------
// registry call back to render my-netdata menu

function toggleExpandIcon(svgEl) {
    if (svgEl.getAttribute('data-icon') === 'caret-down') {
        svgEl.setAttribute('data-icon', 'caret-up');
    } else {
        svgEl.setAttribute('data-icon', 'caret-down');
    }
}

function toggleAgentItem(e, guid) {
    e.stopPropagation();
    e.preventDefault();

    toggleExpandIcon(e.currentTarget.children[0]);

    const el = document.querySelector(`.agent-alternate-urls.agent-${guid}`);
    if (el) {
        el.classList.toggle('collapsed');
    }
}

// TODO: consider renaming to `truncateString`

/// Enforces a maximum string length while retaining the prefix and the postfix of
/// the string.
function clipString(str, maxLength) {
    if (str.length <= maxLength) {
        return str;
    }

    const spanLength = Math.floor((maxLength - 3) / 2);
    return `${str.substring(0, spanLength)}...${str.substring(str.length - spanLength)}`;
}

// When you stream metrics from netdata to netdata, the recieving netdata now 
// has multiple host databases. It's own, and multiple mirrored. Mirrored databases 
// can be accessed with <http://localhost:19999/host/NAME/>
function renderStreamedHosts(options) {
    let html = `<div class="info-item">Databases streamed to this agent</div>`;

    var base = document.location.origin.toString() + document.location.pathname.toString();
    if (base.endsWith("/host/" + options.hostname + "/")) {
        base = base.substring(0, base.length - ("/host/" + options.hostname + "/").toString().length);
    }

    if (base.endsWith("/")) {
        base = base.substring(0, base.length - 1);
    }

    var master = options.hosts[0].hostname;
    var sorted = options.hosts.sort(function (a, b) {
        if (a.hostname === master) {
            return -1;
        }
        return naturalSortCompare(a.hostname, b.hostname);
    });

    for (const s of sorted) {
        let url, icon;
        const hostname = s.hostname;

        if (hostname === master) {
            url = `base${'/'}`;
            icon = 'home';
        } else {
            url = `${base}/host/${hostname}/`;
            icon = 'window-restore';
        }

        html += (
            `<div class="agent-item">
                <a class="registry_link" href="${url}#" onClick="return gotoHostedModalHandler('${url}');">
                    <i class="fas fa-${icon}" style="color: #999;"></i>
                </a>                    
                <a class="registry_link" href="${url}#" onClick="return gotoHostedModalHandler('${url}');">${hostname}</a>
                <div></div>
            </div>`
        )
    }

    return html;
}

function renderMachines(machinesArray) {
    let html = `<div class="info-item">My netdata agents</div>`;

    if (machinesArray === null) {
        let ret = loadLocalStorage("registryCallback");
        if (ret) {
            machinesArray = JSON.parse(ret);
            console.log("failed to contact the registry - loaded registry data from browser local storage");
        }
    }

    let found = false;

    if (machinesArray) {
        saveLocalStorage("registryCallback", JSON.stringify(machinesArray));

        var machines = machinesArray.sort(function (a, b) {
            return naturalSortCompare(a.name, b.name);
        });

        for (const machine of machines) {
            found = true;

            const alternateUrlItems = (
                `<div class="agent-alternate-urls agent-${machine.guid} collapsed">
                ${machine.alternate_urls.reduce(
                    (str, url) => str + (
                        `<div class="agent-item agent-item--alternate">
                            <div></div>
                            <a href="${url}" title="${url}">${clipString(url, 64)}</a>
                            <a href="#" onclick="deleteRegistryModalHandler('${machine.guid}', '${machine.name}', '${url}'); return false;">
                                <i class="fas fa-trash" style="color: #777;"></i>
                            </a>
                        </div>`
                    ),
                    ''
                )}
                </div>`
            )

            html += (
                `<div class="agent-item agent-${machine.guid}">
                    <i class="fas fa-chart-bar" color: #fff"></i>
                    <span>
                        <a class="registry_link" href="${machine.url}#" onClick="return gotoServerModalHandler('${machine.guid}');">${machine.name}</a>
                    </span>
                    <a href="#" onClick="toggleAgentItem(event, '${machine.guid}');">
                        <i class="fas fa-caret-down" style="color: #999"></i>
                    </a>
                </div>
                ${alternateUrlItems}`
            )
        }
    }

    if (!found) {
        if (machines) {
            html += (
                `<div class="info-item">
                    <a href="https://github.com/netdata/netdata/tree/master/registry#netdata-registry" target="_blank">Your netdata server list is empty</a> 
                </div>`
            )
        } else {
            el += '<li></li>';
            html += (
                `<div class="info-item">
                    <a href="https://github.com/netdata/netdata/tree/master/registry#netdata-registry" target="_blank">Failed to contact the registry</a> 
                </div>`
            )
        }

        html += `<hr />`;
        html += `<div class="info-item">Demo netdata agents</div>`;

        const demoServers = [
            {url: "//london.netdata.rocks/default.html", title: "UK - London (DigitalOcean.com)"},
            {url: "//newyork.netdata.rocks/default.html", title: "US - New York (DigitalOcean.com)"},
            {url: "//sanfrancisco.netdata.rocks/default.html", title: "US - San Francisco (DigitalOcean.com)"},
            {url: "//atlanta.netdata.rocks/default.html", title: "US - Atlanta (CDN77.com)"},
            {url: "//frankfurt.netdata.rocks/default.html", title: "Germany - Frankfurt (DigitalOcean.com)"},
            {url: "//toronto.netdata.rocks/default.html", title: "Canada - Toronto (DigitalOcean.com)"},
            {url: "//singapore.netdata.rocks/default.html", title: "Japan - Singapore (DigitalOcean.com)"},
            {url: "//bangalore.netdata.rocks/default.html", title: "India - Bangalore (DigitalOcean.com)"},

        ]

        for (const server of demoServers) {
            html += (
                `<div class="agent-item">
                    <i class="fas fa-chart-bar" color: #fff"></i>
                    <a href="${server.url}">${server.title}</a>
                    <div></div>
                </div>
                `
            );
        }
    }

    return html;
}

// Populates the my-netdata menu.
function netdataRegistryCallback(machinesArray) {
    let html = '';

    if (options.hosts.length > 1) {
        html += renderStreamedHosts(options) + `<hr />`;
    }

    html += renderMachines(machinesArray);

    html += (
        `<hr />
        <div class="agent-item">
            <i class="fas fa-cog""></i>
            <a href="#" onclick="switchRegistryModalHandler(); return false;">Switch Identity</a>
            <div></div>
        </div>
        <div class="agent-item">
            <i class="fas fa-question-circle""></i>
            <a href="https://github.com/netdata/netdata/tree/master/registry#netdata-registry" target="_blank">What is this?</a>
            <div></div>
        </div>`
    )

    const el = document.getElementById('my-netdata-dropdown-content')
    el.classList.add(`theme-${netdataTheme}`);
    el.innerHTML = html;

    gotoServerInit();
};

function isdemo() {
    if (this_is_demo !== null) {
        return this_is_demo;
    }
    this_is_demo = false;

    try {
        if (typeof document.location.hostname === 'string') {
            if (document.location.hostname.endsWith('.my-netdata.io') ||
                document.location.hostname.endsWith('.mynetdata.io') ||
                document.location.hostname.endsWith('.netdata.rocks') ||
                document.location.hostname.endsWith('.netdata.ai') ||
                document.location.hostname.endsWith('.netdata.live') ||
                document.location.hostname.endsWith('.firehol.org') ||
                document.location.hostname.endsWith('.netdata.online') ||
                document.location.hostname.endsWith('.netdata.cloud')) {
                this_is_demo = true;
            }
        }
    }
    catch (error) {
    }
    return this_is_demo;
}

function netdataURL(url, forReload) {
    if (typeof url === 'undefined')
    // url = document.location.toString();
    {
        url = '';
    }

    if (url.indexOf('#') !== -1) {
        url = url.substring(0, url.indexOf('#'));
    }

    var hash = urlOptions.genHash(forReload);

    // console.log('netdataURL: ' + url + hash);

    return url + hash;
}

function netdataReload(url) {
    document.location = verifyURL(netdataURL(url, true));

    // since we play with hash
    // this is needed to reload the page
    location.reload();
}

function gotoHostedModalHandler(url) {
    document.location = verifyURL(url + urlOptions.genHash());
    return false;
}

var gotoServerValidateRemaining = 0;
var gotoServerMiddleClick = false;
var gotoServerStop = false;

function gotoServerValidateUrl(id, guid, url) {
    var penalty = 0;
    var error = 'failed';

    if (document.location.toString().startsWith('http://') && url.toString().startsWith('https://'))
    // we penalize https only if the current url is http
    // to allow the user walk through all its servers.
    {
        penalty = 500;
    } else if (document.location.toString().startsWith('https://') && url.toString().startsWith('http://')) {
        error = 'can\'t check';
    }

    var finalURL = netdataURL(url);

    setTimeout(function () {
        document.getElementById('gotoServerList').innerHTML += '<tr><td style="padding-left: 20px;"><a href="' + verifyURL(finalURL) + '" target="_blank">' + escapeUserInputHTML(url) + '</a></td><td style="padding-left: 30px;"><code id="' + guid + '-' + id + '-status">checking...</code></td></tr>';

        NETDATA.registry.hello(url, function (data) {
            if (typeof data !== 'undefined' && data !== null && typeof data.machine_guid === 'string' && data.machine_guid === guid) {
                // console.log('OK ' + id + ' URL: ' + url);
                document.getElementById(guid + '-' + id + '-status').innerHTML = "OK";

                if (!gotoServerStop) {
                    gotoServerStop = true;

                    if (gotoServerMiddleClick) {
                        window.open(verifyURL(finalURL), '_blank');
                        gotoServerMiddleClick = false;
                        document.getElementById('gotoServerResponse').innerHTML = '<b>Opening new window to ' + NETDATA.registry.machines[guid].name + '<br/><a href="' + verifyURL(finalURL) + '">' + escapeUserInputHTML(url) + '</a></b><br/>(check your pop-up blocker if it fails)';
                    } else {
                        document.getElementById('gotoServerResponse').innerHTML += 'found it! It is at:<br/><small>' + escapeUserInputHTML(url) + '</small>';
                        document.location = verifyURL(finalURL);
                    }
                }
            } else {
                if (typeof data !== 'undefined' && data !== null && typeof data.machine_guid === 'string' && data.machine_guid !== guid) {
                    error = 'wrong machine';
                }

                document.getElementById(guid + '-' + id + '-status').innerHTML = error;
                gotoServerValidateRemaining--;
                if (gotoServerValidateRemaining <= 0) {
                    gotoServerMiddleClick = false;
                    document.getElementById('gotoServerResponse').innerHTML = '<b>Sorry! I cannot find any operational URL for this server</b>';
                }
            }
        });
    }, (id * 50) + penalty);
}

function gotoServerModalHandler(guid) {
    // console.log('goto server: ' + guid);

    gotoServerStop = false;
    var checked = {};
    var len = NETDATA.registry.machines[guid].alternate_urls.length;
    var count = 0;

    document.getElementById('gotoServerResponse').innerHTML = '';
    document.getElementById('gotoServerList').innerHTML = '';
    document.getElementById('gotoServerName').innerHTML = NETDATA.registry.machines[guid].name;
    $('#gotoServerModal').modal('show');

    gotoServerValidateRemaining = len;
    while (len--) {
        var url = NETDATA.registry.machines[guid].alternate_urls[len];
        checked[url] = true;
        gotoServerValidateUrl(count++, guid, url);
    }

    setTimeout(function () {
        if (gotoServerStop === false) {
            document.getElementById('gotoServerResponse').innerHTML = '<b>Added all the known URLs for this machine.</b>';
            NETDATA.registry.search(guid, function (data) {
                // console.log(data);
                len = data.urls.length;
                while (len--) {
                    var url = data.urls[len][1];
                    // console.log(url);
                    if (typeof checked[url] === 'undefined') {
                        gotoServerValidateRemaining++;
                        checked[url] = true;
                        gotoServerValidateUrl(count++, guid, url);
                    }
                }
            });
        }
    }, 2000);
    return false;
}

function gotoServerInit() {
    $(".registry_link").on('click', function (e) {
        if (e.which === 2) {
            e.preventDefault();
            gotoServerMiddleClick = true;
        } else {
            gotoServerMiddleClick = false;
        }

        return true;
    });
}

function switchRegistryModalHandler() {
    document.getElementById('switchRegistryPersonGUID').value = NETDATA.registry.person_guid;
    document.getElementById('switchRegistryURL').innerHTML = NETDATA.registry.server;
    document.getElementById('switchRegistryResponse').innerHTML = '';
    $('#switchRegistryModal').modal('show');
}

function notifyForSwitchRegistry() {
    var n = document.getElementById('switchRegistryPersonGUID').value;

    if (n !== '' && n.length === 36) {
        NETDATA.registry.switch(n, function (result) {
            if (result !== null) {
                $('#switchRegistryModal').modal('hide');
                NETDATA.registry.init();
            } else {
                document.getElementById('switchRegistryResponse').innerHTML = "<b>Sorry! The registry rejected your request.</b>";
            }
        });
    } else {
        document.getElementById('switchRegistryResponse').innerHTML = "<b>The ID you have entered is not a GUID.</b>";
    }
}

var deleteRegistryUrl = null;

function deleteRegistryModalHandler(guid, name, url) {
    void (guid);

    deleteRegistryUrl = url;
    document.getElementById('deleteRegistryServerName').innerHTML = name;
    document.getElementById('deleteRegistryServerName2').innerHTML = name;
    document.getElementById('deleteRegistryServerURL').innerHTML = url;
    document.getElementById('deleteRegistryResponse').innerHTML = '';
    $('#deleteRegistryModal').modal('show');
}

function notifyForDeleteRegistry() {
    if (deleteRegistryUrl) {
        NETDATA.registry.delete(deleteRegistryUrl, function (result) {
            if (result !== null) {
                deleteRegistryUrl = null;
                $('#deleteRegistryModal').modal('hide');
                NETDATA.registry.init();
            } else {
                document.getElementById('deleteRegistryResponse').innerHTML = "<b>Sorry! this command was rejected by the registry server.</b>";
            }
        });
    }
}

var options = {
    menus: {},
    submenu_names: {},
    data: null,
    hostname: 'netdata_server', // will be overwritten by the netdata server
    version: 'unknown',
    hosts: [],

    duration: 0, // the default duration of the charts
    update_every: 1,

    chartsPerRow: 0,
    // chartsMinWidth: 1450,
    chartsHeight: 180,
};

function chartsPerRow(total) {
    void (total);

    if (options.chartsPerRow === 0) {
        return 1;
        //var width = Math.floor(total / options.chartsMinWidth);
        //if(width === 0) width = 1;
        //return width;
    } else {
        return options.chartsPerRow;
    }
}

function prioritySort(a, b) {
    if (a.priority < b.priority) {
        return -1;
    }
    if (a.priority > b.priority) {
        return 1;
    }
    return naturalSortCompare(a.name, b.name);
}

function sortObjectByPriority(object) {
    var idx = {};
    var sorted = [];

    for (var i in object) {
        if (!object.hasOwnProperty(i)) {
            continue;
        }

        if (typeof idx[i] === 'undefined') {
            idx[i] = object[i];
            sorted.push(i);
        }
    }

    sorted.sort(function (a, b) {
        if (idx[a].priority < idx[b].priority) {
            return -1;
        }
        if (idx[a].priority > idx[b].priority) {
            return 1;
        }
        return naturalSortCompare(a, b);
    });

    return sorted;
}

// ----------------------------------------------------------------------------
// scroll to a section, without changing the browser history

function scrollToId(hash) {
    if (hash && hash !== '' && document.getElementById(hash) !== null) {
        var offset = $('#' + hash).offset();
        if (typeof offset !== 'undefined') {
            //console.log('scrolling to ' + hash + ' at ' + offset.top.toString());
            $('html, body').animate({scrollTop: offset.top - 30}, 0);
        }
    }

    // we must return false to prevent the default action
    return false;
}

// ----------------------------------------------------------------------------

// user editable information
var customDashboard = {
    menu: {},
    submenu: {},
    context: {}
};

// netdata standard information
var netdataDashboard = {
    sparklines_registry: {},
    os: 'unknown',

    menu: {},
    submenu: {},
    context: {},

    // generate a sparkline
    // used in the documentation
    sparkline: function (prefix, chart, dimension, units, suffix) {
        if (options.data === null || typeof options.data.charts === 'undefined') {
            return '';
        }

        if (typeof options.data.charts[chart] === 'undefined') {
            return '';
        }

        if (typeof options.data.charts[chart].dimensions === 'undefined') {
            return '';
        }

        if (typeof options.data.charts[chart].dimensions[dimension] === 'undefined') {
            return '';
        }

        var key = chart + '.' + dimension;

        if (typeof units === 'undefined') {
            units = '';
        }

        if (typeof this.sparklines_registry[key] === 'undefined') {
            this.sparklines_registry[key] = {count: 1};
        } else {
            this.sparklines_registry[key].count++;
        }

        key = key + '.' + this.sparklines_registry[key].count;

        return prefix + '<div class="netdata-container" data-netdata="' + chart + '" data-after="-120" data-width="25%" data-height="15px" data-chart-library="dygraph" data-dygraph-theme="sparkline" data-dimensions="' + dimension + '" data-show-value-of-' + dimension + '-at="' + key + '"></div> (<span id="' + key + '" style="display: inline-block; min-width: 50px; text-align: right;">X</span>' + units + ')' + suffix;
    },

    gaugeChart: function (title, width, dimensions, colors) {
        if (typeof colors === 'undefined') {
            colors = '';
        }

        if (typeof dimensions === 'undefined') {
            dimensions = '';
        }

        return '<div class="netdata-container" data-netdata="CHART_UNIQUE_ID"'
            + ' data-dimensions="' + dimensions + '"'
            + ' data-chart-library="gauge"'
            + ' data-gauge-adjust="width"'
            + ' data-title="' + title + '"'
            + ' data-width="' + width + '"'
            + ' data-before="0"'
            + ' data-after="-CHART_DURATION"'
            + ' data-points="CHART_DURATION"'
            + ' data-colors="' + colors + '"'
            + ' role="application"></div>';
    },

    anyAttribute: function (obj, attr, key, def) {
        if (typeof (obj[key]) !== 'undefined') {
            var x = obj[key][attr];

            if (typeof (x) === 'undefined') {
                return def;
            }

            if (typeof (x) === 'function') {
                return x(netdataDashboard.os);
            }

            return x;
        }

        return def;
    },

    menuTitle: function (chart) {
        if (typeof chart.menu_pattern !== 'undefined') {
            return (this.anyAttribute(this.menu, 'title', chart.menu_pattern, chart.menu_pattern).toString()
                + '&nbsp;' + chart.type.slice(-(chart.type.length - chart.menu_pattern.length - 1)).toString()).replace(/_/g, ' ');
        }

        return (this.anyAttribute(this.menu, 'title', chart.menu, chart.menu)).toString().replace(/_/g, ' ');
    },

    menuIcon: function (chart) {
        if (typeof chart.menu_pattern !== 'undefined') {
            return this.anyAttribute(this.menu, 'icon', chart.menu_pattern, '<i class="fas fa-puzzle-piece"></i>').toString();
        }

        return this.anyAttribute(this.menu, 'icon', chart.menu, '<i class="fas fa-puzzle-piece"></i>');
    },

    menuInfo: function (chart) {
        if (typeof chart.menu_pattern !== 'undefined') {
            return this.anyAttribute(this.menu, 'info', chart.menu_pattern, null);
        }

        return this.anyAttribute(this.menu, 'info', chart.menu, null);
    },

    menuHeight: function (chart) {
        if (typeof chart.menu_pattern !== 'undefined') {
            return this.anyAttribute(this.menu, 'height', chart.menu_pattern, 1.0);
        }

        return this.anyAttribute(this.menu, 'height', chart.menu, 1.0);
    },

    submenuTitle: function (menu, submenu) {
        var key = menu + '.' + submenu;
        // console.log(key);
        var title = this.anyAttribute(this.submenu, 'title', key, submenu).toString().replace(/_/g, ' ');
        if (title.length > 28) {
            var a = title.substring(0, 13);
            var b = title.substring(title.length - 12, title.length);
            return a + '...' + b;
        }
        return title;
    },

    submenuInfo: function (menu, submenu) {
        var key = menu + '.' + submenu;
        return this.anyAttribute(this.submenu, 'info', key, null);
    },

    submenuHeight: function (menu, submenu, relative) {
        var key = menu + '.' + submenu;
        return this.anyAttribute(this.submenu, 'height', key, 1.0) * relative;
    },

    contextInfo: function (id) {
        var x = this.anyAttribute(this.context, 'info', id, null);

        if (x !== null) {
            return '<div class="shorten dashboard-context-info netdata-chart-alignment" role="document">' + x + '</div>';
        } else {
            return '';
        }
    },

    contextValueRange: function (id) {
        if (typeof this.context[id] !== 'undefined' && typeof this.context[id].valueRange !== 'undefined') {
            return this.context[id].valueRange;
        } else {
            return '[null, null]';
        }
    },

    contextHeight: function (id, def) {
        if (typeof this.context[id] !== 'undefined' && typeof this.context[id].height !== 'undefined') {
            return def * this.context[id].height;
        } else {
            return def;
        }
    },

    contextDecimalDigits: function (id, def) {
        if (typeof this.context[id] !== 'undefined' && typeof this.context[id].decimalDigits !== 'undefined') {
            return this.context[id].decimalDigits;
        } else {
            return def;
        }
    }
};

// ----------------------------------------------------------------------------

// enrich the data structure returned by netdata
// to reflect our menu system and content
// TODO: this is a shame - we should fix charts naming (issue #807)
function enrichChartData(chart) {
    var parts = chart.type.split('_');
    var tmp = parts[0];

    switch (tmp) {
        case 'ap':
        case 'net':
        case 'disk':
        case 'statsd':
            chart.menu = tmp;
            break;

        case 'apache':
            chart.menu = chart.type;
            if (parts.length > 2 && parts[1] === 'cache') {
                chart.menu_pattern = tmp + '_' + parts[1];
            } else if (parts.length > 1) {
                chart.menu_pattern = tmp;
            }
            break;

        case 'bind':
            chart.menu = chart.type;
            if (parts.length > 2 && parts[1] === 'rndc') {
                chart.menu_pattern = tmp + '_' + parts[1];
            } else if (parts.length > 1) {
                chart.menu_pattern = tmp;
            }
            break;

        case 'cgroup':
            chart.menu = chart.type;
            if (chart.id.match(/.*[\._\/-:]qemu[\._\/-:]*/) || chart.id.match(/.*[\._\/-:]kvm[\._\/-:]*/)) {
                chart.menu_pattern = 'cgqemu';
            } else {
                chart.menu_pattern = 'cgroup';
            }
            break;

        case 'go':
            chart.menu = chart.type;
            if (parts.length > 2 && parts[1] === 'expvar') {
                chart.menu_pattern = tmp + '_' + parts[1];
            } else if (parts.length > 1) {
                chart.menu_pattern = tmp;
            }
            break;

        case 'isc':
            chart.menu = chart.type;
            if (parts.length > 2 && parts[1] === 'dhcpd') {
                chart.menu_pattern = tmp + '_' + parts[1];
            } else if (parts.length > 1) {
                chart.menu_pattern = tmp;
            }
            break;

        case 'ovpn':
            chart.menu = chart.type;
            if (parts.length > 3 && parts[1] === 'status' && parts[2] === 'log') {
                chart.menu_pattern = tmp + '_' + parts[1];
            } else if (parts.length > 1) {
                chart.menu_pattern = tmp;
            }
            break;

        case 'smartd':
        case 'web':
            chart.menu = chart.type;
            if (parts.length > 2 && parts[1] === 'log') {
                chart.menu_pattern = tmp + '_' + parts[1];
            } else if (parts.length > 1) {
                chart.menu_pattern = tmp;
            }
            break;

        case 'tc':
            chart.menu = tmp;

            // find a name for this device from fireqos info
            // we strip '_(in|out)' or '(in|out)_'
            if (chart.context === 'tc.qos' && (typeof options.submenu_names[chart.family] === 'undefined' || options.submenu_names[chart.family] === chart.family)) {
                var n = chart.name.split('.')[1];
                if (n.endsWith('_in')) {
                    options.submenu_names[chart.family] = n.slice(0, n.lastIndexOf('_in'));
                } else if (n.endsWith('_out')) {
                    options.submenu_names[chart.family] = n.slice(0, n.lastIndexOf('_out'));
                } else if (n.startsWith('in_')) {
                    options.submenu_names[chart.family] = n.slice(3, n.length);
                } else if (n.startsWith('out_')) {
                    options.submenu_names[chart.family] = n.slice(4, n.length);
                } else {
                    options.submenu_names[chart.family] = n;
                }
            }

            // increase the priority of IFB devices
            // to have inbound appear before outbound
            if (chart.id.match(/.*-ifb$/)) {
                chart.priority--;
            }

            break;

        default:
            chart.menu = chart.type;
            if (parts.length > 1) {
                chart.menu_pattern = tmp;
            }
            break;
    }

    chart.submenu = chart.family;
}

// ----------------------------------------------------------------------------

function headMain(os, charts, duration) {
    void (os);

    if (urlOptions.mode === 'print') {
        return '';
    }

    var head = '';

    if (typeof charts['system.swap'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.swap"'
            + ' data-dimensions="used"'
            + ' data-append-options="percentage"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="Used Swap"'
            + ' data-units="%"'
            + ' data-easypiechart-max-value="100"'
            + ' data-width="9%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-colors="#DD4400"'
            + ' role="application"></div>';
    }

    if (typeof charts['system.io'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.io"'
            + ' data-dimensions="in"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="Disk Read"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.io.mainhead"'
            + ' role="application"></div>';

        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.io"'
            + ' data-dimensions="out"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="Disk Write"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.io.mainhead"'
            + ' role="application"></div>';
    }
    else if (typeof charts['system.pgpgio'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.pgpgio"'
            + ' data-dimensions="in"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="Disk Read"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.pgpgio.mainhead"'
            + ' role="application"></div>';

        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.pgpgio"'
            + ' data-dimensions="out"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="Disk Write"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.pgpgio.mainhead"'
            + ' role="application"></div>';
    }

    if (typeof charts['system.cpu'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.cpu"'
            + ' data-chart-library="gauge"'
            + ' data-title="CPU"'
            + ' data-units="%"'
            + ' data-gauge-max-value="100"'
            + ' data-width="20%"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-colors="' + NETDATA.colors[12] + '"'
            + ' role="application"></div>';
    }

    if (typeof charts['system.net'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.net"'
            + ' data-dimensions="received"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="Net Inbound"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.net.mainhead"'
            + ' role="application"></div>';

        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.net"'
            + ' data-dimensions="sent"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="Net Outbound"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.net.mainhead"'
            + ' role="application"></div>';
    }
    else if (typeof charts['system.ip'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.ip"'
            + ' data-dimensions="received"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="IP Inbound"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.ip.mainhead"'
            + ' role="application"></div>';

        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.ip"'
            + ' data-dimensions="sent"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="IP Outbound"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.ip.mainhead"'
            + ' role="application"></div>';
    }
    else if (typeof charts['system.ipv4'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.ipv4"'
            + ' data-dimensions="received"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="IPv4 Inbound"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.ipv4.mainhead"'
            + ' role="application"></div>';

        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.ipv4"'
            + ' data-dimensions="sent"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="IPv4 Outbound"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.ipv4.mainhead"'
            + ' role="application"></div>';
    }
    else if (typeof charts['system.ipv6'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.ipv6"'
            + ' data-dimensions="received"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="IPv6 Inbound"'
            + ' data-units="kbps"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.ipv6.mainhead"'
            + ' role="application"></div>';

        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.ipv6"'
            + ' data-dimensions="sent"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="IPv6 Outbound"'
            + ' data-units="kbps"'
            + ' data-width="11%"'
            + ' data-before="0"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-common-units="system.ipv6.mainhead"'
            + ' role="application"></div>';
    }

    if (typeof charts['system.ram'] !== 'undefined') {
        head += '<div class="netdata-container" style="margin-right: 10px;" data-netdata="system.ram"'
            + ' data-dimensions="used|buffers|active|wired"' // active and wired are FreeBSD stats
            + ' data-append-options="percentage"'
            + ' data-chart-library="easypiechart"'
            + ' data-title="Used RAM"'
            + ' data-units="%"'
            + ' data-easypiechart-max-value="100"'
            + ' data-width="9%"'
            + ' data-after="-' + duration.toString() + '"'
            + ' data-points="' + duration.toString() + '"'
            + ' data-colors="' + NETDATA.colors[7] + '"'
            + ' role="application"></div>';
    }

    return head;
}

function generateHeadCharts(type, chart, duration) {
    if (urlOptions.mode === 'print') {
        return '';
    }

    var head = '';
    var hcharts = netdataDashboard.anyAttribute(netdataDashboard.context, type, chart.context, []);
    if (hcharts.length > 0) {
        var hi = 0, hlen = hcharts.length;
        while (hi < hlen) {
            if (typeof hcharts[hi] === 'function') {
                head += hcharts[hi](netdataDashboard.os, chart.id).replace(/CHART_DURATION/g, duration.toString()).replace(/CHART_UNIQUE_ID/g, chart.id);
            } else {
                head += hcharts[hi].replace(/CHART_DURATION/g, duration.toString()).replace(/CHART_UNIQUE_ID/g, chart.id);
            }
            hi++;
        }
    }
    return head;
}

function renderPage(menus, data) {
    var div = document.getElementById('charts_div');
    var pcent_width = Math.floor(100 / chartsPerRow($(div).width()));

    // find the proper duration for per-second updates
    var duration = Math.round(($(div).width() * pcent_width / 100 * data.update_every / 3) / 60) * 60;
    options.duration = duration;
    options.update_every = data.update_every;

    var html = '';
    var sidebar = '<ul class="nav dashboard-sidenav" data-spy="affix" id="sidebar_ul">';
    var mainhead = headMain(netdataDashboard.os, data.charts, duration);

    // sort the menus
    var main = sortObjectByPriority(menus);
    var i = 0, len = main.length;
    while (i < len) {
        var menu = main[i++];

        // generate an entry at the main menu

        var menuid = NETDATA.name2id('menu_' + menu);
        sidebar += '<li class=""><a href="#' + menuid + '" onClick="return scrollToId(\'' + menuid + '\');">' + menus[menu].icon + ' ' + menus[menu].title + '</a><ul class="nav">';
        html += '<div role="section" class="dashboard-section"><div role="sectionhead"><h1 id="' + menuid + '" role="heading">' + menus[menu].icon + ' ' + menus[menu].title + '</h1></div><div role="section"  class="dashboard-subsection">';

        if (menus[menu].info !== null) {
            html += menus[menu].info;
        }

        // console.log(' >> ' + menu + ' (' + menus[menu].priority + '): ' + menus[menu].title);

        var shtml = '';
        var mhead = '<div class="netdata-chart-row">' + mainhead;
        mainhead = '';

        // sort the submenus of this menu
        var sub = sortObjectByPriority(menus[menu].submenus);
        var si = 0, slen = sub.length;
        while (si < slen) {
            var submenu = sub[si++];

            // generate an entry at the submenu
            var submenuid = NETDATA.name2id('menu_' + menu + '_submenu_' + submenu);
            sidebar += '<li class><a href="#' + submenuid + '" onClick="return scrollToId(\'' + submenuid + '\');">' + menus[menu].submenus[submenu].title + '</a></li>';
            shtml += '<div role="section" class="dashboard-section-container" id="' + submenuid + '"><h2 id="' + submenuid + '" class="netdata-chart-alignment" role="heading">' + menus[menu].submenus[submenu].title + '</h2>';

            if (menus[menu].submenus[submenu].info !== null) {
                shtml += '<div class="dashboard-submenu-info netdata-chart-alignment" role="document">' + menus[menu].submenus[submenu].info + '</div>';
            }

            var head = '<div class="netdata-chart-row">';
            var chtml = '';

            // console.log('    \------- ' + submenu + ' (' + menus[menu].submenus[submenu].priority + '): ' + menus[menu].submenus[submenu].title);

            // sort the charts in this submenu of this menu
            menus[menu].submenus[submenu].charts.sort(prioritySort);
            var ci = 0, clen = menus[menu].submenus[submenu].charts.length;
            while (ci < clen) {
                var chart = menus[menu].submenus[submenu].charts[ci++];

                // generate the submenu heading charts
                mhead += generateHeadCharts('mainheads', chart, duration);
                head += generateHeadCharts('heads', chart, duration);

                function chartCommonMin(family, context, units) {
                    var x = netdataDashboard.anyAttribute(netdataDashboard.context, 'commonMin', context, undefined);
                    if (typeof x !== 'undefined') {
                        return ' data-common-min="' + family + '/' + context + '/' + units + '"';
                    } else {
                        return '';
                    }
                }

                function chartCommonMax(family, context, units) {
                    var x = netdataDashboard.anyAttribute(netdataDashboard.context, 'commonMax', context, undefined);
                    if (typeof x !== 'undefined') {
                        return ' data-common-max="' + family + '/' + context + '/' + units + '"';
                    } else {
                        return '';
                    }
                }

                // generate the chart
                if (urlOptions.mode === 'print') {
                    chtml += '<div role="row" class="dashboard-print-row">';
                }

                chtml += '<div class="netdata-chartblock-container" style="width: ' + pcent_width.toString() + '%;">' + netdataDashboard.contextInfo(chart.context) + '<div class="netdata-container" id="chart_' + NETDATA.name2id(chart.id) + '" data-netdata="' + chart.id + '"'
                    + ' data-width="100%"'
                    + ' data-height="' + netdataDashboard.contextHeight(chart.context, options.chartsHeight).toString() + 'px"'
                    + ' data-dygraph-valuerange="' + netdataDashboard.contextValueRange(chart.context) + '"'
                    + ' data-before="0"'
                    + ' data-after="-' + duration.toString() + '"'
                    + ' data-id="' + NETDATA.name2id(options.hostname + '/' + chart.id) + '"'
                    + ' data-colors="' + netdataDashboard.anyAttribute(netdataDashboard.context, 'colors', chart.context, '') + '"'
                    + ' data-decimal-digits="' + netdataDashboard.contextDecimalDigits(chart.context, -1) + '"'
                    + chartCommonMin(chart.family, chart.context, chart.units)
                    + chartCommonMax(chart.family, chart.context, chart.units)
                    + ' role="application"></div></div>';

                if (urlOptions.mode === 'print') {
                    chtml += '</div>';
                }

                // console.log('         \------- ' + chart.id + ' (' + chart.priority + '): ' + chart.context  + ' height: ' + menus[menu].submenus[submenu].height);
            }

            head += '</div>';
            shtml += head + chtml + '</div>';
        }

        mhead += '</div>';
        sidebar += '</ul></li>';
        html += mhead + shtml + '</div></div><hr role="separator"/>';
    }

    sidebar += '<li class="" style="padding-top:15px;"><a href="https://github.com/netdata/netdata/blob/master/doc/Add-more-charts-to-netdata.md#add-more-charts-to-netdata" target="_blank"><i class="fas fa-plus"></i> add more charts</a></li>';
    sidebar += '<li class=""><a href="https://github.com/netdata/netdata/tree/master/health#Health-monitoring" target="_blank"><i class="fas fa-plus"></i> add more alarms</a></li>';
    sidebar += '<li class="" style="margin:20px;color:#666;"><small>netdata on <b>' + data.hostname.toString() + '</b>, collects every ' + ((data.update_every === 1) ? 'second' : data.update_every.toString() + ' seconds') + ' <b>' + data.dimensions_count.toLocaleString() + '</b> metrics, presented as <b>' + data.charts_count.toLocaleString() + '</b> charts and monitored by <b>' + data.alarms_count.toLocaleString() + '</b> alarms, using ' + Math.round(data.rrd_memory_bytes / 1024 / 1024).toLocaleString() + ' MB of memory for ' + NETDATA.seconds4human(data.update_every * data.history, {space: '&nbsp;'}) + ' of real-time history.<br/>&nbsp;<br/><b>netdata</b><br/>v' + data.version.toString() + '</small></li>';
    sidebar += '</ul>';
    div.innerHTML = html;
    document.getElementById('sidebar').innerHTML = sidebar;

    if (urlOptions.highlight === true) {
        NETDATA.globalChartUnderlay.init(null
            , urlOptions.highlight_after
            , urlOptions.highlight_before
            , (urlOptions.after > 0) ? urlOptions.after : null
            , (urlOptions.before > 0) ? urlOptions.before : null
        );
    } else {
        NETDATA.globalChartUnderlay.clear();
    }

    if (urlOptions.mode === 'print') {
        printPage();
    } else {
        finalizePage();
    }
}

function renderChartsAndMenu(data) {
    options.menus = {};
    options.submenu_names = {};

    var menus = options.menus;
    var charts = data.charts;
    var m, menu_key;

    for (var c in charts) {
        if (!charts.hasOwnProperty(c)) {
            continue;
        }

        var chart = charts[c];
        enrichChartData(chart);
        m = chart.menu;

        // create the menu
        if (typeof menus[m] === 'undefined') {
            menus[m] = {
                menu_pattern: chart.menu_pattern,
                priority: chart.priority,
                submenus: {},
                title: netdataDashboard.menuTitle(chart),
                icon: netdataDashboard.menuIcon(chart),
                info: netdataDashboard.menuInfo(chart),
                height: netdataDashboard.menuHeight(chart) * options.chartsHeight
            };
        } else {
            if (typeof (menus[m].menu_pattern) === 'undefined') {
                menus[m].menu_pattern = chart.menu_pattern;
            }

            if (chart.priority < menus[m].priority) {
                menus[m].priority = chart.priority;
            }
        }

        menu_key = (typeof (menus[m].menu_pattern) !== 'undefined') ? menus[m].menu_pattern : m;

        // create the submenu
        if (typeof menus[m].submenus[chart.submenu] === 'undefined') {
            menus[m].submenus[chart.submenu] = {
                priority: chart.priority,
                charts: [],
                title: null,
                info: netdataDashboard.submenuInfo(menu_key, chart.submenu),
                height: netdataDashboard.submenuHeight(menu_key, chart.submenu, menus[m].height)
            };
        } else {
            if (chart.priority < menus[m].submenus[chart.submenu].priority) {
                menus[m].submenus[chart.submenu].priority = chart.priority;
            }
        }

        // index the chart in the menu/submenu
        menus[m].submenus[chart.submenu].charts.push(chart);
    }

    // propagate the descriptive subname given to QoS
    // to all the other submenus with the same name
    for (m in menus) {
        if (!menus.hasOwnProperty(m)) {
            continue;
        }

        for (var s in menus[m].submenus) {
            if (!menus[m].submenus.hasOwnProperty(s)) {
                continue;
            }

            // set the family using a name
            if (typeof options.submenu_names[s] !== 'undefined') {
                menus[m].submenus[s].title = s + ' (' + options.submenu_names[s] + ')';
            } else {
                menu_key = (typeof (menus[m].menu_pattern) !== 'undefined') ? menus[m].menu_pattern : m;
                menus[m].submenus[s].title = netdataDashboard.submenuTitle(menu_key, s);
            }
        }
    }

    renderPage(menus, data);
}

// ----------------------------------------------------------------------------

function loadJs(url, callback) {
    $.ajax({
        url: url,
        cache: true,
        dataType: "script",
        xhrFields: {withCredentials: true} // required for the cookie
    })
        .fail(function () {
            alert('Cannot load required JS library: ' + url);
        })
        .always(function () {
            if (typeof callback === 'function') {
                callback();
            }
        })
}

var clipboardLoaded = false;

function loadClipboard(callback) {
    if (clipboardLoaded === false) {
        clipboardLoaded = true;
        loadJs('lib/clipboard-polyfill-be05dad.js', callback);
    } else {
        callback();
    }
}

var bootstrapTableLoaded = false;

function loadBootstrapTable(callback) {
    if (bootstrapTableLoaded === false) {
        bootstrapTableLoaded = true;
        loadJs('lib/bootstrap-table-1.11.0.min.js', function () {
            loadJs('lib/bootstrap-table-export-1.11.0.min.js', function () {
                loadJs('lib/tableExport-1.6.0.min.js', callback);
            })
        });
    } else {
        callback();
    }
}

var bootstrapSliderLoaded = false;

function loadBootstrapSlider(callback) {
    if (bootstrapSliderLoaded === false) {
        bootstrapSliderLoaded = true;
        loadJs('lib/bootstrap-slider-10.0.0.min.js', function () {
            NETDATA._loadCSS('css/bootstrap-slider-10.0.0.min.css');
            callback();
        });
    } else {
        callback();
    }
}

var lzStringLoaded = false;

function loadLzString(callback) {
    if (lzStringLoaded === false) {
        lzStringLoaded = true;
        loadJs('lib/lz-string-1.4.4.min.js', callback);
    } else {
        callback();
    }
}

var pakoLoaded = false;

function loadPako(callback) {
    if (pakoLoaded === false) {
        pakoLoaded = true;
        loadJs('lib/pako-1.0.6.min.js', callback);
    } else {
        callback();
    }
}

// ----------------------------------------------------------------------------

function clipboardCopy(text) {
    clipboard.writeText(text);
}

function clipboardCopyBadgeEmbed(url) {
    clipboard.writeText('<embed src="' + url + '" type="image/svg+xml" height="20"/>');
}

// ----------------------------------------------------------------------------

function alarmsUpdateModal() {
    var active = '<h3>Raised Alarms</h3><table class="table">';
    var all = '<h3>All Running Alarms</h3><div class="panel-group" id="alarms_all_accordion" role="tablist" aria-multiselectable="true">';
    var footer = '<hr/><a href="https://github.com/netdata/netdata/tree/master/web/api/badges#netdata-badges" target="_blank">netdata badges</a> refresh automatically. Their color indicates the state of the alarm: <span style="color: #e05d44"><b>&nbsp;red&nbsp;</b></span> is critical, <span style="color:#fe7d37"><b>&nbsp;orange&nbsp;</b></span> is warning, <span style="color: #4c1"><b>&nbsp;bright green&nbsp;</b></span> is ok, <span style="color: #9f9f9f"><b>&nbsp;light grey&nbsp;</b></span> is undefined (i.e. no data or no status), <span style="color: #000"><b>&nbsp;black&nbsp;</b></span> is not initialized. You can copy and paste their URLs to embed them in any web page.<br/>netdata can send notifications for these alarms. Check <a href="https://github.com/netdata/netdata/blob/master/health/notifications/health_alarm_notify.conf">this configuration file</a> for more information.';

    loadClipboard(function () {
    });

    NETDATA.alarms.get('all', function (data) {
        options.alarm_families = [];

        alarmsCallback(data);

        if (data === null) {
            document.getElementById('alarms_active').innerHTML =
                document.getElementById('alarms_all').innerHTML =
                    document.getElementById('alarms_log').innerHTML =
                        'failed to load alarm data!';
            return;
        }

        function alarmid4human(id) {
            if (id === 0) {
                return '-';
            }

            return id.toString();
        }

        function timestamp4human(timestamp, space) {
            if (timestamp === 0) {
                return '-';
            }

            if (typeof space === 'undefined') {
                space = '&nbsp;';
            }

            var t = new Date(timestamp * 1000);
            var now = new Date();

            if (t.toDateString() === now.toDateString()) {
                return t.toLocaleTimeString();
            }

            return t.toLocaleDateString() + space + t.toLocaleTimeString();
        }

        function alarm_lookup_explain(alarm, chart) {
            var dimensions = ' of all values ';

            if (chart.dimensions.length > 1) {
                dimensions = ' of the sum of all dimensions ';
            }

            if (typeof alarm.lookup_dimensions !== 'undefined') {
                var d = alarm.lookup_dimensions.replace(/|/g, ',');
                var x = d.split(',');
                if (x.length > 1) {
                    dimensions = 'of the sum of dimensions <code>' + alarm.lookup_dimensions + '</code> ';
                } else {
                    dimensions = 'of all values of dimension <code>' + alarm.lookup_dimensions + '</code> ';
                }
            }

            return '<code>' + alarm.lookup_method + '</code> '
                + dimensions + ', of chart <code>' + alarm.chart + '</code>'
                + ', starting <code>' + NETDATA.seconds4human(alarm.lookup_after + alarm.lookup_before, {space: '&nbsp;'}) + '</code> and up to <code>' + NETDATA.seconds4human(alarm.lookup_before, {space: '&nbsp;'}) + '</code>'
                + ((alarm.lookup_options) ? (', with options <code>' + alarm.lookup_options.replace(/ /g, ',&nbsp;') + '</code>') : '')
                + '.';
        }

        function alarm_to_html(alarm, full) {
            var chart = options.data.charts[alarm.chart];
            if (typeof (chart) === 'undefined') {
                chart = options.data.charts_by_name[alarm.chart];
                if (typeof (chart) === 'undefined') {
                    // this means the charts loaded are incomplete
                    // probably netdata was restarted and more alarms
                    // are now available.
                    console.log('Cannot find chart ' + alarm.chart + ', you probably need to refresh the page.');
                    return '';
                }
            }

            var has_alarm = (typeof alarm.warn !== 'undefined' || typeof alarm.crit !== 'undefined');
            var badge_url = NETDATA.alarms.server + '/api/v1/badge.svg?chart=' + alarm.chart + '&alarm=' + alarm.name + '&refresh=auto';

            var action_buttons = '<br/>&nbsp;<br/>role: <b>' + alarm.recipient + '</b><br/>&nbsp;<br/>'
                + '<div class="action-button ripple" title="click to scroll the dashboard to the chart of this alarm" data-toggle="tooltip" data-placement="bottom" onClick="scrollToChartAfterHidingModal(\'' + alarm.chart + '\'); $(\'#alarmsModal\').modal(\'hide\'); return false;"><i class="fab fa-periscope"></i></div>'
                + '<div class="action-button ripple" title="click to copy to the clipboard the URL of this badge" data-toggle="tooltip" data-placement="bottom" onClick="clipboardCopy(\'' + badge_url + '\'); return false;"><i class="far fa-copy"></i></div>'
                + '<div class="action-button ripple" title="click to copy to the clipboard an auto-refreshing <code>embed</code> html element for this badge" data-toggle="tooltip" data-placement="bottom" onClick="clipboardCopyBadgeEmbed(\'' + badge_url + '\'); return false;"><i class="fas fa-copy"></i></div>';

            var html = '<tr><td class="text-center" style="vertical-align:middle" width="40%"><b>' + alarm.chart + '</b><br/>&nbsp;<br/><embed src="' + badge_url + '" type="image/svg+xml" height="20"/><br/>&nbsp;<br/><span style="font-size: 18px">' + alarm.info + '</span>' + action_buttons + '</td>'
                + '<td><table class="table">'
                + ((typeof alarm.warn !== 'undefined') ? ('<tr><td width="10%" style="text-align:right">warning&nbsp;when</td><td><span style="font-family: monospace; color:#fe7d37; font-weight: bold;">' + alarm.warn + '</span></td></tr>') : '')
                + ((typeof alarm.crit !== 'undefined') ? ('<tr><td width="10%" style="text-align:right">critical&nbsp;when</td><td><span style="font-family: monospace; color: #e05d44; font-weight: bold;">' + alarm.crit + '</span></td></tr>') : '');

            if (full === true) {
                var units = chart.units;
                if (units === '%') {
                    units = '&#37;';
                }

                html += ((typeof alarm.lookup_after !== 'undefined') ? ('<tr><td width="10%" style="text-align:right">db&nbsp;lookup</td><td>' + alarm_lookup_explain(alarm, chart) + '</td></tr>') : '')
                    + ((typeof alarm.calc !== 'undefined') ? ('<tr><td width="10%" style="text-align:right">calculation</td><td><span style="font-family: monospace;">' + alarm.calc + '</span></td></tr>') : '')
                    + ((chart.green !== null) ? ('<tr><td width="10%" style="text-align:right">green&nbsp;threshold</td><td><code>' + chart.green + ' ' + units + '</code></td></tr>') : '')
                    + ((chart.red !== null) ? ('<tr><td width="10%" style="text-align:right">red&nbsp;threshold</td><td><code>' + chart.red + ' ' + units + '</code></td></tr>') : '');
            }

            var delay = '';
            if ((alarm.delay_up_duration > 0 || alarm.delay_down_duration > 0) && alarm.delay_multiplier !== 0 && alarm.delay_max_duration > 0) {
                if (alarm.delay_up_duration === alarm.delay_down_duration) {
                    delay += '<small><br/>hysteresis ' + NETDATA.seconds4human(alarm.delay_up_duration, {
                        space: '&nbsp;',
                        negative_suffix: ''
                    });
                } else {
                    delay = '<small><br/>hysteresis ';
                    if (alarm.delay_up_duration > 0) {
                        delay += 'on&nbsp;escalation&nbsp;<code>' + NETDATA.seconds4human(alarm.delay_up_duration, {
                            space: '&nbsp;',
                            negative_suffix: ''
                        }) + '</code>, ';
                    }
                    if (alarm.delay_down_duration > 0) {
                        delay += 'on&nbsp;recovery&nbsp;<code>' + NETDATA.seconds4human(alarm.delay_down_duration, {
                            space: '&nbsp;',
                            negative_suffix: ''
                        }) + '</code>, ';
                    }
                }
                if (alarm.delay_multiplier !== 1.0) {
                    delay += 'multiplied&nbsp;by&nbsp;<code>' + alarm.delay_multiplier.toString() + '</code>';
                    delay += ',&nbsp;up&nbsp;to&nbsp;<code>' + NETDATA.seconds4human(alarm.delay_max_duration, {
                        space: '&nbsp;',
                        negative_suffix: ''
                    }) + '</code>';
                }
                delay += '</small>';
            }

            html += '<tr><td width="10%" style="text-align:right">check&nbsp;every</td><td>' + NETDATA.seconds4human(alarm.update_every, {
                    space: '&nbsp;',
                    negative_suffix: ''
                }) + '</td></tr>'
                + ((has_alarm === true) ? ('<tr><td width="10%" style="text-align:right">execute</td><td><span style="font-family: monospace;">' + alarm.exec + '</span>' + delay + '</td></tr>') : '')
                + '<tr><td width="10%" style="text-align:right">source</td><td><span style="font-family: monospace;">' + alarm.source + '</span></td></tr>'
                + '</table></td></tr>';

            return html;
        }

        function alarm_family_show(id) {
            var html = '<table class="table">';
            var family = options.alarm_families[id];
            var len = family.arr.length;
            while (len--) {
                var alarm = family.arr[len];
                html += alarm_to_html(alarm, true);
            }
            html += '</table>';

            $('#alarm_all_' + id.toString()).html(html);
            enableTooltipsAndPopovers();
        }

        // find the proper family of each alarm
        var x, family, alarm;
        var count_active = 0;
        var count_all = 0;
        var families = {};
        var families_sort = [];
        for (x in data.alarms) {
            if (!data.alarms.hasOwnProperty(x)) {
                continue;
            }

            alarm = data.alarms[x];
            family = alarm.family;

            // find the chart
            var chart = options.data.charts[alarm.chart];
            if (typeof chart === 'undefined') {
                chart = options.data.charts_by_name[alarm.chart];
            }

            // not found - this should never happen!
            if (typeof chart === 'undefined') {
                console.log('WARNING: alarm ' + x + ' is linked to chart ' + alarm.chart + ', which is not found in the list of chart got from the server.');
                chart = {priority: 9999999};
            }
            else if (typeof chart.menu !== 'undefined' && typeof chart.submenu !== 'undefined')
            // the family based on the chart
            {
                family = chart.menu + ' - ' + chart.submenu;
            }

            if (typeof families[family] === 'undefined') {
                families[family] = {
                    name: family,
                    arr: [],
                    priority: chart.priority
                };

                families_sort.push(families[family]);
            }

            if (chart.priority < families[family].priority) {
                families[family].priority = chart.priority;
            }

            families[family].arr.unshift(alarm);
        }

        // sort the families, like the dashboard menu does
        var families_sorted = families_sort.sort(function (a, b) {
            if (a.priority < b.priority) {
                return -1;
            }
            if (a.priority > b.priority) {
                return 1;
            }
            return naturalSortCompare(a.name, b.name);
        });

        var i = 0;
        var fc = 0;
        var len = families_sorted.length;
        while (len--) {
            family = families_sorted[i++].name;
            var active_family_added = false;
            var expanded = 'true';
            var collapsed = '';
            var cin = 'in';

            if (fc !== 0) {
                all += "</table></div></div></div>";
                expanded = 'false';
                collapsed = 'class="collapsed"';
                cin = '';
            }

            all += '<div class="panel panel-default"><div class="panel-heading" role="tab" id="alarm_all_heading_' + fc.toString() + '"><h4 class="panel-title"><a ' + collapsed + ' role="button" data-toggle="collapse" data-parent="#alarms_all_accordion" href="#alarm_all_' + fc.toString() + '" aria-expanded="' + expanded + '" aria-controls="alarm_all_' + fc.toString() + '">' + family.toString() + '</a></h4></div><div id="alarm_all_' + fc.toString() + '" class="panel-collapse collapse ' + cin + '" role="tabpanel" aria-labelledby="alarm_all_heading_' + fc.toString() + '" data-alarm-id="' + fc.toString() + '"><div class="panel-body" id="alarm_all_body_' + fc.toString() + '">';

            options.alarm_families[fc] = families[family];

            fc++;

            var arr = families[family].arr;
            var c = arr.length;
            while (c--) {
                alarm = arr[c];
                if (alarm.status === 'WARNING' || alarm.status === 'CRITICAL') {
                    if (!active_family_added) {
                        active_family_added = true;
                        active += '<tr><th class="text-center" colspan="2"><h4>' + family + '</h4></th></tr>';
                    }
                    count_active++;
                    active += alarm_to_html(alarm, true);
                }

                count_all++;
            }
        }
        active += "</table>";
        if (families_sorted.length > 0) {
            all += "</div></div></div>";
        }
        all += "</div>";

        if (!count_active) {
            active += '<div style="width:100%; height: 100px; text-align: center;"><span style="font-size: 50px;"><i class="fas fa-thumbs-up"></i></span><br/>Everything is normal. No raised alarms.</div>';
        } else {
            active += footer;
        }

        if (!count_all) {
            all += "<h4>No alarms are running in this system.</h4>";
        } else {
            all += footer;
        }

        document.getElementById('alarms_active').innerHTML = active;
        document.getElementById('alarms_all').innerHTML = all;
        enableTooltipsAndPopovers();

        if (families_sorted.length > 0) {
            alarm_family_show(0);
        }

        // register bootstrap events
        var $accordion = $('#alarms_all_accordion');
        $accordion.on('show.bs.collapse', function (d) {
            var target = $(d.target);
            var id = $(target).data('alarm-id');
            alarm_family_show(id);
        });
        $accordion.on('hidden.bs.collapse', function (d) {
            var target = $(d.target);
            var id = $(target).data('alarm-id');
            $('#alarm_all_' + id.toString()).html('');
        });

        document.getElementById('alarms_log').innerHTML = '<h3>Alarm Log</h3><table id="alarms_log_table"></table>';

        loadBootstrapTable(function () {
            $('#alarms_log_table').bootstrapTable({
                url: NETDATA.alarms.server + '/api/v1/alarm_log?all',
                cache: false,
                pagination: true,
                pageSize: 10,
                showPaginationSwitch: false,
                search: true,
                searchTimeOut: 300,
                searchAlign: 'left',
                showColumns: true,
                showExport: true,
                exportDataType: 'basic',
                exportOptions: {
                    fileName: 'netdata_alarm_log'
                },
                rowStyle: function (row, index) {
                    void (index);

                    switch (row.status) {
                        case 'CRITICAL':
                            return {classes: 'danger'};
                            break;
                        case 'WARNING':
                            return {classes: 'warning'};
                            break;
                        case 'UNDEFINED':
                            return {classes: 'info'};
                            break;
                        case 'CLEAR':
                            return {classes: 'success'};
                            break;
                    }
                    return {};
                },
                showFooter: false,
                showHeader: true,
                showRefresh: true,
                showToggle: false,
                sortable: true,
                silentSort: false,
                columns: [
                    {
                        field: 'when',
                        title: 'Event Date',
                        valign: 'middle',
                        titleTooltip: 'The date and time the even took place',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return timestamp4human(value, ' ');
                        },
                        align: 'center',
                        switchable: false,
                        sortable: true
                    },
                    {
                        field: 'hostname',
                        title: 'Host',
                        valign: 'middle',
                        titleTooltip: 'The host that generated this event',
                        align: 'center',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'unique_id',
                        title: 'Unique ID',
                        titleTooltip: 'The host unique ID for this event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return alarmid4human(value);
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'alarm_id',
                        title: 'Alarm ID',
                        titleTooltip: 'The ID of the alarm that generated this event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return alarmid4human(value);
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'alarm_event_id',
                        title: 'Alarm Event ID',
                        titleTooltip: 'The incremental ID of this event for the given alarm',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return alarmid4human(value);
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'chart',
                        title: 'Chart',
                        titleTooltip: 'The chart the alarm is attached to',
                        align: 'center',
                        valign: 'middle',
                        switchable: false,
                        sortable: true
                    },
                    {
                        field: 'family',
                        title: 'Family',
                        titleTooltip: 'The family of the chart the alarm is attached to',
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'name',
                        title: 'Alarm',
                        titleTooltip: 'The alarm name that generated this event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return value.toString().replace(/_/g, ' ');
                        },
                        align: 'center',
                        valign: 'middle',
                        switchable: false,
                        sortable: true
                    },
                    {
                        field: 'value_string',
                        title: 'Friendly Value',
                        titleTooltip: 'The value of the alarm, that triggered this event',
                        align: 'right',
                        valign: 'middle',
                        sortable: true
                    },
                    {
                        field: 'old_value_string',
                        title: 'Friendly Old Value',
                        titleTooltip: 'The value of the alarm, just before this event',
                        align: 'right',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'old_value',
                        title: 'Old Value',
                        titleTooltip: 'The value of the alarm, just before this event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return ((value !== null) ? Math.round(value * 100) / 100 : 'NaN').toString();
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'value',
                        title: 'Value',
                        titleTooltip: 'The value of the alarm, that triggered this event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return ((value !== null) ? Math.round(value * 100) / 100 : 'NaN').toString();
                        },
                        align: 'right',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'units',
                        title: 'Units',
                        titleTooltip: 'The units of the value of the alarm',
                        align: 'left',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'old_status',
                        title: 'Old Status',
                        titleTooltip: 'The status of the alarm, just before this event',
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'status',
                        title: 'Status',
                        titleTooltip: 'The status of the alarm, that was set due to this event',
                        align: 'center',
                        valign: 'middle',
                        switchable: false,
                        sortable: true
                    },
                    {
                        field: 'duration',
                        title: 'Last Duration',
                        titleTooltip: 'The duration the alarm was at its previous state, just before this event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return NETDATA.seconds4human(value, {negative_suffix: '', space: ' ', now: 'no time'});
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'non_clear_duration',
                        title: 'Raised Duration',
                        titleTooltip: 'The duration the alarm was raised, just before this event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return NETDATA.seconds4human(value, {negative_suffix: '', space: ' ', now: 'no time'});
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'recipient',
                        title: 'Recipient',
                        titleTooltip: 'The recipient of this event',
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'processed',
                        title: 'Processed Status',
                        titleTooltip: 'True when this event is processed',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);

                            if (value === true) {
                                return 'DONE';
                            } else {
                                return 'PENDING';
                            }
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'updated',
                        title: 'Updated Status',
                        titleTooltip: 'True when this event has been updated by another event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);

                            if (value === true) {
                                return 'UPDATED';
                            } else {
                                return 'CURRENT';
                            }
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'updated_by_id',
                        title: 'Updated By ID',
                        titleTooltip: 'The unique ID of the event that obsoleted this one',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return alarmid4human(value);
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'updates_id',
                        title: 'Updates ID',
                        titleTooltip: 'The unique ID of the event obsoleted because of this event',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return alarmid4human(value);
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'exec',
                        title: 'Script',
                        titleTooltip: 'The script to handle the event notification',
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'exec_run',
                        title: 'Script Run At',
                        titleTooltip: 'The date and time the script has been ran',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return timestamp4human(value, ' ');
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'exec_code',
                        title: 'Script Return Value',
                        titleTooltip: 'The return code of the script',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);

                            if (value === 0) {
                                return 'OK (returned 0)';
                            } else {
                                return 'FAILED (with code ' + value.toString() + ')';
                            }
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'delay',
                        title: 'Script Delay',
                        titleTooltip: 'The hysteresis of the notification',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);

                            return NETDATA.seconds4human(value, {negative_suffix: '', space: ' ', now: 'no time'});
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'delay_up_to_timestamp',
                        title: 'Script Delay Run At',
                        titleTooltip: 'The date and time the script should be run, after hysteresis',
                        formatter: function (value, row, index) {
                            void (row);
                            void (index);
                            return timestamp4human(value, ' ');
                        },
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'info',
                        title: 'Description',
                        titleTooltip: 'A short description of the alarm',
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    },
                    {
                        field: 'source',
                        title: 'Alarm Source',
                        titleTooltip: 'The source of configuration of the alarm',
                        align: 'center',
                        valign: 'middle',
                        visible: false,
                        sortable: true
                    }
                ]
            });
            // console.log($('#alarms_log_table').bootstrapTable('getOptions'));
        });
    });
}

function alarmsCallback(data) {
    var count = 0, x;
    for (x in data.alarms) {
        if (!data.alarms.hasOwnProperty(x)) {
            continue;
        }

        var alarm = data.alarms[x];
        if (alarm.status === 'WARNING' || alarm.status === 'CRITICAL') {
            count++;
        }
    }

    if (count > 0) {
        document.getElementById('alarms_count_badge').innerHTML = count.toString();
    } else {
        document.getElementById('alarms_count_badge').innerHTML = '';
    }
}

function initializeDynamicDashboardWithData(data) {
    if (data !== null) {
        options.hostname = data.hostname;
        options.data = data;
        options.version = data.version;
        netdataDashboard.os = data.os;

        if (typeof data.hosts !== 'undefined') {
            options.hosts = data.hosts;
        }

        // update the dashboard hostname
        document.getElementById('hostname').innerHTML = options.hostname + ((netdataSnapshotData !== null) ? ' (snap)' : '').toString();
        document.getElementById('hostname').href = NETDATA.serverDefault;
        document.getElementById('netdataVersion').innerHTML = options.version;

        if (netdataSnapshotData !== null) {
            $('#alarmsButton').hide();
            $('#updateButton').hide();
            // $('#loadButton').hide();
            $('#saveButton').hide();
            $('#printButton').hide();
        }

        // update the dashboard title
        document.title = options.hostname + ' netdata dashboard';

        // close the splash screen
        $("#loadOverlay").css("display", "none");

        // create a chart_by_name index
        data.charts_by_name = {};
        var charts = data.charts;
        var x;
        for (x in charts) {
            if (!charts.hasOwnProperty(x)) {
                continue;
            }

            var chart = charts[x];
            data.charts_by_name[chart.name] = chart;
        }

        // render all charts
        renderChartsAndMenu(data);
    }
}

// an object to keep initilization configuration
// needed due to the async nature of the XSS modal
var initializeConfig = {
    url: null,
    custom_info: true,
};

function loadCustomDashboardInfo(url, callback) {
    loadJs(url, function () {
        $.extend(true, netdataDashboard, customDashboard);
        callback();
    });
}

function initializeChartsAndCustomInfo() {
    NETDATA.alarms.callback = alarmsCallback;

    // download all the charts the server knows
    NETDATA.chartRegistry.downloadAll(initializeConfig.url, function (data) {
        if (data !== null) {
            if (initializeConfig.custom_info === true && typeof data.custom_info !== 'undefined' && data.custom_info !== "" && netdataSnapshotData === null) {
                //console.log('loading custom dashboard decorations from server ' + initializeConfig.url);
                loadCustomDashboardInfo(NETDATA.serverDefault + data.custom_info, function () {
                    initializeDynamicDashboardWithData(data);
                });
            } else {
                //console.log('not loading custom dashboard decorations from server ' + initializeConfig.url);
                initializeDynamicDashboardWithData(data);
            }
        }
    });
}

function xssModalDisableXss() {
    //console.log('disabling xss checks');
    NETDATA.xss.enabled = false;
    NETDATA.xss.enabled_for_data = false;
    initializeConfig.custom_info = true;
    initializeChartsAndCustomInfo();
    return false;
}

function xssModalKeepXss() {
    //console.log('keeping xss checks');
    NETDATA.xss.enabled = true;
    NETDATA.xss.enabled_for_data = true;
    initializeConfig.custom_info = false;
    initializeChartsAndCustomInfo();
    return false;
}

function initializeDynamicDashboard(netdata_url) {
    if (typeof netdata_url === 'undefined' || netdata_url === null) {
        netdata_url = NETDATA.serverDefault;
    }

    initializeConfig.url = netdata_url;

    // initialize clickable alarms
    NETDATA.alarms.chart_div_offset = -50;
    NETDATA.alarms.chart_div_id_prefix = 'chart_';
    NETDATA.alarms.chart_div_animation_duration = 0;

    NETDATA.pause(function () {
        if (typeof netdataCheckXSS !== 'undefined' && netdataCheckXSS === true) {
            //$("#loadOverlay").css("display","none");
            document.getElementById('netdataXssModalServer').innerText = initializeConfig.url;
            $('#xssModal').modal('show');
        } else {
            initializeChartsAndCustomInfo();
        }
    });
}

// ----------------------------------------------------------------------------

function versionLog(msg) {
    document.getElementById('versionCheckLog').innerHTML = msg;
}

function getNetdataCommitIdFromVersion() {
    var s = options.version.split('-');

    if (s.length !== 3) {
        return null;
    }
    if (s[2][0] === 'g') {
        var v = s[2].split('_')[0].substring(1, 8);
        if (v.length === 7) {
            versionLog('Installed git commit id of netdata is ' + v);
            document.getElementById('netdataCommitId').innerHTML = v;
            return v;
        }
    }
    return null;
}

function getNetdataCommitId(force, callback) {
    versionLog('Downloading installed git commit id from netdata...');

    $.ajax({
        url: 'version.txt',
        async: true,
        cache: false,
        xhrFields: {withCredentials: true} // required for the cookie
    })
        .done(function (data) {
            data = data.replace(/(\r\n|\n|\r| |\t)/gm, "");

            var c = getNetdataCommitIdFromVersion();
            if (c !== null && data.length === 40 && data.substring(0, 7) !== c) {
                versionLog('Installed files commit id and internal netdata git commit id do not match');
                data = c;
            }

            if (data.length >= 7) {
                versionLog('Installed git commit id of netdata is ' + data);
                document.getElementById('netdataCommitId').innerHTML = data.substring(0, 7);
                callback(data);
            }
        })
        .fail(function () {
            versionLog('Failed to download installed git commit id from netdata!');

            if (force === true) {
                var c = getNetdataCommitIdFromVersion();
                if (c === null) {
                    versionLog('Cannot find the git commit id of netdata.');
                }
                callback(c);
            } else {
                callback(null);
            }
        });
}

function getGithubLatestCommit(callback) {
    versionLog('Downloading latest git commit id info from github...');

    $.ajax({
        url: 'https://api.github.com/repos/netdata/netdata/commits',
        async: true,
        cache: false
    })
        .done(function (data) {
            versionLog('Latest git commit id from github is ' + data[0].sha);
            callback(data[0].sha);
        })
        .fail(function () {
            versionLog('Failed to download installed git commit id from github!');
            callback(null);
        });
}

function checkForUpdate(force, callback) {
    getNetdataCommitId(force, function (sha1) {
        if (sha1 === null) {
            callback(null, null);
        }

        getGithubLatestCommit(function (sha2) {
            callback(sha1, sha2);
        });
    });

    return null;
}

function notifyForUpdate(force) {
    versionLog('<p>checking for updates...</p>');

    var now = Date.now();

    if (typeof force === 'undefined' || force !== true) {
        var last = loadLocalStorage('last_update_check');

        if (typeof last === 'string') {
            last = parseInt(last);
        } else {
            last = 0;
        }

        if (now - last < 3600000 * 8) {
            // no need to check it - too soon
            return;
        }
    }

    checkForUpdate(force, function (sha1, sha2) {
        var save = false;

        if (sha1 === null) {
            save = false;
            versionLog('<p><big>Failed to get your netdata git commit id!</big></p><p>You can always get the latest netdata from <a href="https://github.com/netdata/netdata" target="_blank">its github page</a>.</p>');
        } else if (sha2 === null) {
            save = false;
            versionLog('<p><big>Failed to get the latest git commit id from github.</big></p><p>You can always get the latest netdata from <a href="https://github.com/netdata/netdata" target="_blank">its github page</a>.</p>');
        } else if (sha1 === sha2) {
            save = true;
            versionLog('<p><big>You already have the latest netdata!</big></p><p>No update yet?<br/>Probably, we need some motivation to keep going on!</p><p>If you haven\'t already, <a href="https://github.com/netdata/netdata" target="_blank">give netdata a <b><i class="fas fa-star"></i></b> at its github page</a>.</p>');
        } else {
            save = true;
            var compare = 'https://github.com/netdata/netdata/compare/' + sha1.toString() + '...' + sha2.toString();

            versionLog('<p><big><strong>New version of netdata available!</strong></big></p><p>Latest commit: <b><code>' + sha2.substring(0, 7).toString() + '</code></b></p><p><a href="' + compare + '" target="_blank">Click here for the changes log</a> since your installed version, and<br/><a href="https://github.com/netdata/netdata/tree/master/installer/UPDATE.md" target="_blank">click here for directions on updating</a> your netdata installation.</p><p>We suggest to review the changes log for new features you may be interested, or important bug fixes you may need.<br/>Keeping your netdata updated, is generally a good idea.</p>');

            document.getElementById('update_badge').innerHTML = '!';
        }

        if (save) {
            saveLocalStorage('last_update_check', now.toString());
        }
    });
}

// ----------------------------------------------------------------------------
// printing dashboards

function showPageFooter() {
    document.getElementById('footer').style.display = 'block';
}

function printPreflight() {
    var url = document.location.origin.toString() + document.location.pathname.toString() + document.location.search.toString() + urlOptions.genHash() + ';mode=print';
    var width = 990;
    var height = screen.height * 90 / 100;
    //console.log(url);
    //console.log(document.location);
    window.open(url, '', 'width=' + width.toString() + ',height=' + height.toString() + ',menubar=no,toolbar=no,personalbar=no,location=no,resizable=no,scrollbars=yes,status=no,chrome=yes,centerscreen=yes,attention=yes,dialog=yes');
    $('#printPreflightModal').modal('hide');
}

function printPage() {
    var print_is_rendering = true;

    $('#printModal').on('hide.bs.modal', function (e) {
        if (print_is_rendering === true) {
            e.preventDefault();
            return false;
        }

        return true;
    });

    $('#printModal').on('show.bs.modal', function () {
        var print_options = {
            stop_updates_when_focus_is_lost: false,
            update_only_visible: false,
            sync_selection: false,
            eliminate_zero_dimensions: false,
            pan_and_zoom_data_padding: false,
            show_help: false,
            legend_toolbox: false,
            resize_charts: false,
            pixels_per_point: 1
        };

        var x;
        for (x in print_options) {
            if (print_options.hasOwnProperty(x)) {
                NETDATA.options.current[x] = print_options[x];
            }
        }

        NETDATA.parseDom();
        showPageFooter();

        NETDATA.globalSelectionSync.stop();
        NETDATA.globalPanAndZoom.setMaster(NETDATA.options.targets[0], urlOptions.after, urlOptions.before);
        // NETDATA.onresize();

        var el = document.getElementById('printModalProgressBar');
        var eltxt = document.getElementById('printModalProgressBarText');

        function update_chart(idx) {
            var state = NETDATA.options.targets[--idx];

            var pcent = (NETDATA.options.targets.length - idx) * 100 / NETDATA.options.targets.length;
            $(el).css('width', pcent + '%').attr('aria-valuenow', pcent);
            eltxt.innerText = Math.round(pcent).toString() + '%, ' + state.id;

            setTimeout(function () {
                state.updateChart(function () {
                    NETDATA.options.targets[idx].resizeForPrint();

                    if (idx > 0) {
                        update_chart(idx);
                    } else {
                        print_is_rendering = false;
                        $('#printModal').modal('hide');
                        window.print();
                        window.close();
                    }
                })
            }, 0);
        }

        print_is_rendering = true;
        update_chart(NETDATA.options.targets.length);
    });

    $('#printModal').modal('show');
}

// --------------------------------------------------------------------

function jsonStringifyFn(obj) {
    return JSON.stringify(obj, function (key, value) {
        return (typeof value === 'function') ? value.toString() : value;
    });
}

function jsonParseFn(str) {
    return JSON.parse(str, function (key, value) {
        if (typeof value != 'string') {
            return value;
        }
        return (value.substring(0, 8) == 'function') ? eval('(' + value + ')') : value;
    });
}

// --------------------------------------------------------------------

var snapshotOptions = {
    bytes_per_chart: 2048,
    compressionDefault: 'pako.deflate.base64',

    compressions: {
        'none': {
            bytes_per_point_memory: 5.2,
            bytes_per_point_disk: 5.6,

            compress: function (s) {
                return s;
            },

            compressed_length: function (s) {
                return s.length;
            },

            uncompress: function (s) {
                return s;
            }
        },

        'pako.deflate.base64': {
            bytes_per_point_memory: 1.8,
            bytes_per_point_disk: 1.9,

            compress: function (s) {
                return btoa(pako.deflate(s, {to: 'string'}));
            },

            compressed_length: function (s) {
                return s.length;
            },

            uncompress: function (s) {
                return pako.inflate(atob(s), {to: 'string'});
            }
        },

        'pako.deflate': {
            bytes_per_point_memory: 1.4,
            bytes_per_point_disk: 3.2,

            compress: function (s) {
                return pako.deflate(s, {to: 'string'});
            },

            compressed_length: function (s) {
                return s.length;
            },

            uncompress: function (s) {
                return pako.inflate(s, {to: 'string'});
            }
        },

        'lzstring.utf16': {
            bytes_per_point_memory: 1.7,
            bytes_per_point_disk: 2.6,

            compress: function (s) {
                return LZString.compressToUTF16(s);
            },

            compressed_length: function (s) {
                return s.length * 2;
            },

            uncompress: function (s) {
                return LZString.decompressFromUTF16(s);
            }
        },

        'lzstring.base64': {
            bytes_per_point_memory: 2.1,
            bytes_per_point_disk: 2.3,

            compress: function (s) {
                return LZString.compressToBase64(s);
            },

            compressed_length: function (s) {
                return s.length;
            },

            uncompress: function (s) {
                return LZString.decompressFromBase64(s);
            }
        },

        'lzstring.uri': {
            bytes_per_point_memory: 2.1,
            bytes_per_point_disk: 2.3,

            compress: function (s) {
                return LZString.compressToEncodedURIComponent(s);
            },

            compressed_length: function (s) {
                return s.length;
            },

            uncompress: function (s) {
                return LZString.decompressFromEncodedURIComponent(s);
            }
        }
    }
};

// --------------------------------------------------------------------
// loading snapshots

function loadSnapshotModalLog(priority, msg) {
    document.getElementById('loadSnapshotStatus').className = "alert alert-" + priority;
    document.getElementById('loadSnapshotStatus').innerHTML = msg;
}

var tmpSnapshotData = null;

function loadSnapshot() {
    $('#loadSnapshotImport').addClass('disabled');

    if (tmpSnapshotData === null) {
        loadSnapshotPreflightEmpty();
        loadSnapshotModalLog('danger', 'no data have been loaded');
        return;
    }

    loadPako(function () {
        loadLzString(function () {
            loadSnapshotModalLog('info', 'Please wait, activating snapshot...');
            $('#loadSnapshotModal').modal('hide');

            netdataShowAlarms = false;
            netdataRegistry = false;
            netdataServer = tmpSnapshotData.server;
            NETDATA.serverDefault = netdataServer;

            document.getElementById('charts_div').innerHTML = '';
            document.getElementById('sidebar').innerHTML = '';
            NETDATA.globalReset();

            if (typeof tmpSnapshotData.hash !== 'undefined') {
                urlOptions.hash = tmpSnapshotData.hash;
            } else {
                urlOptions.hash = '#';
            }

            if (typeof tmpSnapshotData.info !== 'undefined') {
                var info = jsonParseFn(tmpSnapshotData.info);
                if (typeof info.menu !== 'undefined') {
                    netdataDashboard.menu = info.menu;
                }

                if (typeof info.submenu !== 'undefined') {
                    netdataDashboard.submenu = info.submenu;
                }

                if (typeof info.context !== 'undefined') {
                    netdataDashboard.context = info.context;
                }
            }

            if (typeof tmpSnapshotData.compression !== 'string') {
                tmpSnapshotData.compression = 'none';
            }

            if (typeof snapshotOptions.compressions[tmpSnapshotData.compression] === 'undefined') {
                alert('unknown compression method: ' + tmpSnapshotData.compression);
                tmpSnapshotData.compression = 'none';
            }

            tmpSnapshotData.uncompress = snapshotOptions.compressions[tmpSnapshotData.compression].uncompress;
            netdataSnapshotData = tmpSnapshotData;

            urlOptions.after = tmpSnapshotData.after_ms;
            urlOptions.before = tmpSnapshotData.before_ms;

            if (typeof tmpSnapshotData.highlight_after_ms !== 'undefined'
                && tmpSnapshotData.highlight_after_ms !== null
                && tmpSnapshotData.highlight_after_ms > 0
                && typeof tmpSnapshotData.highlight_before_ms !== 'undefined'
                && tmpSnapshotData.highlight_before_ms !== null
                && tmpSnapshotData.highlight_before_ms > 0
            ) {
                urlOptions.highlight_after = tmpSnapshotData.highlight_after_ms;
                urlOptions.highlight_before = tmpSnapshotData.highlight_before_ms;
                urlOptions.highlight = true;
            } else {
                urlOptions.highlight_after = 0;
                urlOptions.highlight_before = 0;
                urlOptions.highlight = false;
            }

            netdataCheckXSS = false; // disable the modal - this does not affect XSS checks, since dashboard.js is already loaded
            NETDATA.xss.enabled = true;             // we should not do any remote requests, but if we do, check them
            NETDATA.xss.enabled_for_data = true;    // check also snapshot data - that have been excluded from the initial check, due to compression
            loadSnapshotPreflightEmpty();
            initializeDynamicDashboard();
        });
    });
};

function loadSnapshotPreflightFile(file) {
    var filename = NETDATA.xss.string(file.name);
    var fr = new FileReader();
    fr.onload = function (e) {
        document.getElementById('loadSnapshotFilename').innerHTML = filename;
        var result = null;
        try {
            result = NETDATA.xss.checkAlways('snapshot', JSON.parse(e.target.result), /^(snapshot\.info|snapshot\.data)$/);

            //console.log(result);
            var date_after = new Date(result.after_ms);
            var date_before = new Date(result.before_ms);

            if (typeof result.charts_ok === 'undefined') {
                result.charts_ok = 'unknown';
            }

            if (typeof result.charts_failed === 'undefined') {
                result.charts_failed = 0;
            }

            if (typeof result.compression === 'undefined') {
                result.compression = 'none';
            }

            if (typeof result.data_size === 'undefined') {
                result.data_size = 0;
            }

            document.getElementById('loadSnapshotFilename').innerHTML = '<code>' + filename + '</code>';
            document.getElementById('loadSnapshotHostname').innerHTML = '<b>' + result.hostname + '</b>, netdata version: <b>' + result.netdata_version.toString() + '</b>';
            document.getElementById('loadSnapshotURL').innerHTML = result.url;
            document.getElementById('loadSnapshotCharts').innerHTML = result.charts.charts_count.toString() + ' charts, ' + result.charts.dimensions_count.toString() + ' dimensions, ' + result.data_points.toString() + ' points per dimension, ' + Math.round(result.duration_ms / result.data_points).toString() + ' ms per point';
            document.getElementById('loadSnapshotInfo').innerHTML = 'version: <b>' + result.snapshot_version.toString() + '</b>, includes <b>' + result.charts_ok.toString() + '</b> unique chart data queries ' + ((result.charts_failed > 0) ? ('<b>' + result.charts_failed.toString() + '</b> failed') : '').toString() + ', compressed with <code>' + result.compression.toString() + '</code>, data size ' + (Math.round(result.data_size * 100 / 1024 / 1024) / 100).toString() + ' MB';
            document.getElementById('loadSnapshotTimeRange').innerHTML = '<b>' + NETDATA.dateTime.localeDateString(date_after) + ' ' + NETDATA.dateTime.localeTimeString(date_after) + '</b> to <b>' + NETDATA.dateTime.localeDateString(date_before) + ' ' + NETDATA.dateTime.localeTimeString(date_before) + '</b>';
            document.getElementById('loadSnapshotComments').innerHTML = ((result.comments) ? result.comments : '').toString();
            loadSnapshotModalLog('success', 'File loaded, click <b>Import</b> to render it!');
            $('#loadSnapshotImport').removeClass('disabled');

            tmpSnapshotData = result;
        }
        catch (e) {
            console.log(e);
            document.getElementById('loadSnapshotStatus').className = "alert alert-danger";
            document.getElementById('loadSnapshotStatus').innerHTML = "Failed to parse this file!";
            $('#loadSnapshotImport').addClass('disabled');
        }
    }

    //console.log(file);
    fr.readAsText(file);
};

function loadSnapshotPreflightEmpty() {
    document.getElementById('loadSnapshotFilename').innerHTML = '';
    document.getElementById('loadSnapshotHostname').innerHTML = '';
    document.getElementById('loadSnapshotURL').innerHTML = '';
    document.getElementById('loadSnapshotCharts').innerHTML = '';
    document.getElementById('loadSnapshotInfo').innerHTML = '';
    document.getElementById('loadSnapshotTimeRange').innerHTML = '';
    document.getElementById('loadSnapshotComments').innerHTML = '';
    loadSnapshotModalLog('success', 'Browse for a snapshot file (or drag it and drop it here), then click <b>Import</b> to render it.');
    $('#loadSnapshotImport').addClass('disabled');
};

var loadSnapshotDragAndDropInitialized = false;

function loadSnapshotDragAndDropSetup() {
    if (loadSnapshotDragAndDropInitialized === false) {
        loadSnapshotDragAndDropInitialized = true;
        $('#loadSnapshotDragAndDrop')
            .on('drag dragstart dragend dragover dragenter dragleave drop', function (e) {
                e.preventDefault();
                e.stopPropagation();
            })
            .on('drop', function (e) {
                if (e.originalEvent.dataTransfer.files.length) {
                    loadSnapshotPreflightFile(e.originalEvent.dataTransfer.files.item(0));
                } else {
                    loadSnapshotPreflightEmpty();
                    loadSnapshotModalLog('danger', 'No file selected');
                }
            });
    }
};

function loadSnapshotPreflight() {
    var files = document.getElementById('loadSnapshotSelectFiles').files;
    if (files.length <= 0) {
        loadSnapshotPreflightEmpty();
        loadSnapshotModalLog('danger', 'No file selected');
        return;
    }

    loadSnapshotModalLog('info', 'Loading file...');

    loadSnapshotPreflightFile(files.item(0));
}

// --------------------------------------------------------------------
// saving snapshots

var saveSnapshotStop = false;

function saveSnapshotCancel() {
    saveSnapshotStop = true;
}

var saveSnapshotModalInitialized = false;

function saveSnapshotModalSetup() {
    if (saveSnapshotModalInitialized === false) {
        saveSnapshotModalInitialized = true;
        $('#saveSnapshotModal')
            .on('hide.bs.modal', saveSnapshotCancel)
            .on('show.bs.modal', saveSnapshotModalInit)
            .on('shown.bs.modal', function () {
                $('#saveSnapshotResolutionSlider').find(".slider-handle:first").attr("tabindex", 1);
                document.getElementById('saveSnapshotComments').focus();
            });
    }
};

function saveSnapshotModalLog(priority, msg) {
    document.getElementById('saveSnapshotStatus').className = "alert alert-" + priority;
    document.getElementById('saveSnapshotStatus').innerHTML = msg;
}

function saveSnapshotModalShowExpectedSize() {
    var points = Math.round(saveSnapshotViewDuration / saveSnapshotSelectedSecondsPerPoint);
    var priority = 'info';
    var msg = 'A moderate snapshot.';

    var sizemb = Math.round(
        (options.data.charts_count * snapshotOptions.bytes_per_chart
            + options.data.dimensions_count * points * snapshotOptions.compressions[saveSnapshotCompression].bytes_per_point_disk)
        * 10 / 1024 / 1024) / 10;

    var memmb = Math.round(
        (options.data.charts_count * snapshotOptions.bytes_per_chart
            + options.data.dimensions_count * points * snapshotOptions.compressions[saveSnapshotCompression].bytes_per_point_memory)
        * 10 / 1024 / 1024) / 10;

    if (sizemb < 10) {
        priority = 'success';
        msg = 'A nice small snapshot!';
    }
    if (sizemb > 50) {
        priority = 'warning';
        msg = 'Will stress your browser...';
    }
    if (sizemb > 100) {
        priority = 'danger';
        msg = 'Hm... good luck...';
    }

    saveSnapshotModalLog(priority, 'The snapshot will have ' + points.toString() + ' points per dimension. Expected size on disk ' + sizemb + ' MB, at browser memory ' + memmb + ' MB.<br/>' + msg);
}

var saveSnapshotCompression = snapshotOptions.compressionDefault;

function saveSnapshotSetCompression(name) {
    saveSnapshotCompression = name;
    document.getElementById('saveSnapshotCompressionName').innerHTML = saveSnapshotCompression;
    saveSnapshotModalShowExpectedSize();
}

var saveSnapshotSlider = null;
var saveSnapshotSelectedSecondsPerPoint = 1;
var saveSnapshotViewDuration = 1;

function saveSnapshotModalInit() {
    $('#saveSnapshotModalProgressSection').hide();
    $('#saveSnapshotResolutionRadio').show();
    saveSnapshotModalLog('info', 'Select resolution and click <b>Save</b>');
    $('#saveSnapshotExport').removeClass('disabled');

    loadBootstrapSlider(function () {
        saveSnapshotViewDuration = options.duration;
        var start_ms = Math.round(Date.now() - saveSnapshotViewDuration * 1000);

        if (NETDATA.globalPanAndZoom.isActive() === true) {
            saveSnapshotViewDuration = Math.round((NETDATA.globalPanAndZoom.force_before_ms - NETDATA.globalPanAndZoom.force_after_ms) / 1000);
            start_ms = NETDATA.globalPanAndZoom.force_after_ms;
        }

        var start_date = new Date(start_ms);
        var yyyymmddhhssmm = start_date.getFullYear() + NETDATA.zeropad(start_date.getMonth() + 1) + NETDATA.zeropad(start_date.getDate()) + '-' + NETDATA.zeropad(start_date.getHours()) + NETDATA.zeropad(start_date.getMinutes()) + NETDATA.zeropad(start_date.getSeconds());

        document.getElementById('saveSnapshotFilename').value = 'netdata-' + options.hostname.toString() + '-' + yyyymmddhhssmm.toString() + '-' + saveSnapshotViewDuration.toString() + '.snapshot';
        saveSnapshotSetCompression(saveSnapshotCompression);

        var min = options.update_every;
        var max = Math.round(saveSnapshotViewDuration / 100);

        if (NETDATA.globalPanAndZoom.isActive() === false) {
            max = Math.round(saveSnapshotViewDuration / 50);
        }

        var view = Math.round(saveSnapshotViewDuration / Math.round($(document.getElementById('charts_div')).width() / 2));

        // console.log('view duration: ' + saveSnapshotViewDuration + ', min: ' + min + ', max: ' + max + ', view: ' + view);

        if (max < 10) {
            max = 10;
        }
        if (max < min) {
            max = min;
        }
        if (view < min) {
            view = min;
        }
        if (view > max) {
            view = max;
        }

        if (saveSnapshotSlider !== null) {
            saveSnapshotSlider.destroy();
        }

        saveSnapshotSlider = new Slider('#saveSnapshotResolutionSlider', {
            ticks: [min, view, max],
            min: min,
            max: max,
            step: options.update_every,
            value: view,
            scale: (max > 100) ? 'logarithmic' : 'linear',
            tooltip: 'always',
            formatter: function (value) {
                if (value < 1) {
                    value = 1;
                }

                if (value < options.data.update_every) {
                    value = options.data.update_every;
                }

                saveSnapshotSelectedSecondsPerPoint = value;
                saveSnapshotModalShowExpectedSize();

                var seconds = ' seconds ';
                if (value === 1) {
                    seconds = ' second ';
                }

                return value + seconds + 'per point' + ((value === options.data.update_every) ? ', server default' : '').toString();
            }
        });
    });
}

function saveSnapshot() {
    loadPako(function () {
        loadLzString(function () {
            saveSnapshotStop = false;
            $('#saveSnapshotModalProgressSection').show();
            $('#saveSnapshotResolutionRadio').hide();
            $('#saveSnapshotExport').addClass('disabled');

            var filename = document.getElementById('saveSnapshotFilename').value;
            // console.log(filename);
            saveSnapshotModalLog('info', 'Generating snapshot as <code>' + filename.toString() + '</code>');

            var save_options = {
                stop_updates_when_focus_is_lost: false,
                update_only_visible: false,
                sync_selection: false,
                eliminate_zero_dimensions: true,
                pan_and_zoom_data_padding: false,
                show_help: false,
                legend_toolbox: false,
                resize_charts: false,
                pixels_per_point: 1
            };
            var backedup_options = {};

            var x;
            for (x in save_options) {
                if (save_options.hasOwnProperty(x)) {
                    backedup_options[x] = NETDATA.options.current[x];
                    NETDATA.options.current[x] = save_options[x];
                }
            }

            var el = document.getElementById('saveSnapshotModalProgressBar');
            var eltxt = document.getElementById('saveSnapshotModalProgressBarText');

            options.data.charts_by_name = null;

            var saveData = {
                hostname: options.hostname,
                server: NETDATA.serverDefault,
                netdata_version: options.data.version,
                snapshot_version: 1,
                after_ms: Date.now() - options.duration * 1000,
                before_ms: Date.now(),
                highlight_after_ms: urlOptions.highlight_after,
                highlight_before_ms: urlOptions.highlight_before,
                duration_ms: options.duration * 1000,
                update_every_ms: options.update_every * 1000,
                data_points: 0,
                url: ((urlOptions.server !== null) ? urlOptions.server : document.location.origin.toString() + document.location.pathname.toString() + document.location.search.toString()).toString(),
                comments: document.getElementById('saveSnapshotComments').value.toString(),
                hash: urlOptions.hash,
                charts: options.data,
                info: jsonStringifyFn({
                    menu: netdataDashboard.menu,
                    submenu: netdataDashboard.submenu,
                    context: netdataDashboard.context
                }),
                charts_ok: 0,
                charts_failed: 0,
                compression: saveSnapshotCompression,
                data_size: 0,
                data: {}
            };

            if (typeof snapshotOptions.compressions[saveData.compression] === 'undefined') {
                alert('unknown compression method: ' + saveData.compression);
                saveData.compression = 'none';
            }

            var compress = snapshotOptions.compressions[saveData.compression].compress;
            var compressed_length = snapshotOptions.compressions[saveData.compression].compressed_length;

            function pack_api1_v1_chart_data(state) {
                if (state.library_name === null || state.data === null) {
                    return;
                }

                var data = state.data;
                state.data = null;
                data.state = null;
                var str = JSON.stringify(data);

                if (typeof str === 'string') {
                    var cstr = compress(str);
                    saveData.data[state.chartDataUniqueID()] = cstr;
                    return compressed_length(cstr);
                } else {
                    return 0;
                }
            }

            var clearPanAndZoom = false;
            if (NETDATA.globalPanAndZoom.isActive() === false) {
                NETDATA.globalPanAndZoom.setMaster(NETDATA.options.targets[0], saveData.after_ms, saveData.before_ms);
                clearPanAndZoom = true;
            }

            saveData.after_ms = NETDATA.globalPanAndZoom.force_after_ms;
            saveData.before_ms = NETDATA.globalPanAndZoom.force_before_ms;
            saveData.duration_ms = saveData.before_ms - saveData.after_ms;
            saveData.data_points = Math.round((saveData.before_ms - saveData.after_ms) / (saveSnapshotSelectedSecondsPerPoint * 1000));
            saveSnapshotModalLog('info', 'Generating snapshot with ' + saveData.data_points.toString() + ' data points per dimension...');

            var charts_count = 0;
            var charts_ok = 0;
            var charts_failed = 0;

            function saveSnapshotRestore() {
                $('#saveSnapshotModal').modal('hide');

                // restore the options
                var x;
                for (x in backedup_options) {
                    if (backedup_options.hasOwnProperty(x)) {
                        NETDATA.options.current[x] = backedup_options[x];
                    }
                }

                $(el).css('width', '0%').attr('aria-valuenow', 0);
                eltxt.innerText = '0%';

                if (clearPanAndZoom) {
                    NETDATA.globalPanAndZoom.clearMaster();
                }

                NETDATA.options.force_data_points = 0;
                NETDATA.options.fake_chart_rendering = false;
                NETDATA.onscroll_updater_enabled = true;
                NETDATA.onresize();
                NETDATA.unpause();

                $('#saveSnapshotExport').removeClass('disabled');
            }

            NETDATA.globalSelectionSync.stop();
            NETDATA.options.force_data_points = saveData.data_points;
            NETDATA.options.fake_chart_rendering = true;
            NETDATA.onscroll_updater_enabled = false;
            NETDATA.abortAllRefreshes();

            var size = 0;
            var info = ' Resolution: <b>' + saveSnapshotSelectedSecondsPerPoint.toString() + ((saveSnapshotSelectedSecondsPerPoint === 1) ? ' second ' : ' seconds ').toString() + 'per point</b>.';

            function update_chart(idx) {
                if (saveSnapshotStop === true) {
                    saveSnapshotModalLog('info', 'Cancelled!');
                    saveSnapshotRestore();
                    return;
                }

                var state = NETDATA.options.targets[--idx];

                var pcent = (NETDATA.options.targets.length - idx) * 100 / NETDATA.options.targets.length;
                $(el).css('width', pcent + '%').attr('aria-valuenow', pcent);
                eltxt.innerText = Math.round(pcent).toString() + '%, ' + state.id;

                setTimeout(function () {
                    charts_count++;
                    state.isVisible(true);
                    state.current.force_after_ms = saveData.after_ms;
                    state.current.force_before_ms = saveData.before_ms;

                    state.updateChart(function (status, reason) {
                        state.current.force_after_ms = null;
                        state.current.force_before_ms = null;

                        if (status === true) {
                            charts_ok++;
                            // state.log('ok');
                            size += pack_api1_v1_chart_data(state);
                        } else {
                            charts_failed++;
                            state.log('failed to be updated: ' + reason);
                        }

                        saveSnapshotModalLog((charts_failed) ? 'danger' : 'info', 'Generated snapshot data size <b>' + (Math.round(size * 100 / 1024 / 1024) / 100).toString() + ' MB</b>. ' + ((charts_failed) ? (charts_failed.toString() + ' charts have failed to be downloaded') : '').toString() + info);

                        if (idx > 0) {
                            update_chart(idx);
                        } else {
                            saveData.charts_ok = charts_ok;
                            saveData.charts_failed = charts_failed;
                            saveData.data_size = size;
                            // console.log(saveData.compression + ': ' + (size / (options.data.dimensions_count * Math.round(saveSnapshotViewDuration / saveSnapshotSelectedSecondsPerPoint))).toString());

                            // save it
                            // console.log(saveData);
                            saveObjectToClient(saveData, filename);

                            if (charts_failed > 0) {
                                alert(charts_failed.toString() + ' failed to be downloaded');
                            }

                            saveSnapshotRestore();
                            saveData = null;
                        }
                    })
                }, 0);
            }

            update_chart(NETDATA.options.targets.length);
        });
    });
}

// --------------------------------------------------------------------
// activate netdata on the page

function dashboardSettingsSetup() {
    var update_options_modal = function () {
        // console.log('update_options_modal');

        var sync_option = function (option) {
            var self = $('#' + option);

            if (self.prop('checked') !== NETDATA.getOption(option)) {
                // console.log('switching ' + option.toString());
                self.bootstrapToggle(NETDATA.getOption(option) ? 'on' : 'off');
            }
        };

        var theme_sync_option = function (option) {
            var self = $('#' + option);

            self.bootstrapToggle(netdataTheme === 'slate' ? 'on' : 'off');
        };
        var units_sync_option = function (option) {
            var self = $('#' + option);

            if (self.prop('checked') !== (NETDATA.getOption('units') === 'auto')) {
                self.bootstrapToggle(NETDATA.getOption('units') === 'auto' ? 'on' : 'off');
            }

            if (self.prop('checked') === true) {
                $('#settingsLocaleTempRow').show();
                $('#settingsLocaleTimeRow').show();
            } else {
                $('#settingsLocaleTempRow').hide();
                $('#settingsLocaleTimeRow').hide();
            }
        };
        var temp_sync_option = function (option) {
            var self = $('#' + option);

            if (self.prop('checked') !== (NETDATA.getOption('temperature') === 'celsius')) {
                self.bootstrapToggle(NETDATA.getOption('temperature') === 'celsius' ? 'on' : 'off');
            }
        };
        var timezone_sync_option = function (option) {
            var self = $('#' + option);

            document.getElementById('browser_timezone').innerText = NETDATA.options.browser_timezone;
            document.getElementById('server_timezone').innerText = NETDATA.options.server_timezone;
            document.getElementById('current_timezone').innerText = (NETDATA.options.current.timezone === 'default') ? 'unset, using browser default' : NETDATA.options.current.timezone;

            if (self.prop('checked') === NETDATA.dateTime.using_timezone) {
                self.bootstrapToggle(NETDATA.dateTime.using_timezone ? 'off' : 'on');
            }
        };

        sync_option('eliminate_zero_dimensions');
        sync_option('destroy_on_hide');
        sync_option('async_on_scroll');
        sync_option('parallel_refresher');
        sync_option('concurrent_refreshes');
        sync_option('sync_selection');
        sync_option('sync_pan_and_zoom');
        sync_option('stop_updates_when_focus_is_lost');
        sync_option('smooth_plot');
        sync_option('pan_and_zoom_data_padding');
        sync_option('show_help');
        sync_option('seconds_as_time');
        theme_sync_option('netdata_theme_control');
        units_sync_option('units_conversion');
        temp_sync_option('units_temp');
        timezone_sync_option('local_timezone');

        if (NETDATA.getOption('parallel_refresher') === false) {
            $('#concurrent_refreshes_row').hide();
        } else {
            $('#concurrent_refreshes_row').show();
        }
    };
    NETDATA.setOption('setOptionCallback', update_options_modal);

    // handle options changes
    $('#eliminate_zero_dimensions').change(function () {
        NETDATA.setOption('eliminate_zero_dimensions', $(this).prop('checked'));
    });
    $('#destroy_on_hide').change(function () {
        NETDATA.setOption('destroy_on_hide', $(this).prop('checked'));
    });
    $('#async_on_scroll').change(function () {
        NETDATA.setOption('async_on_scroll', $(this).prop('checked'));
    });
    $('#parallel_refresher').change(function () {
        NETDATA.setOption('parallel_refresher', $(this).prop('checked'));
    });
    $('#concurrent_refreshes').change(function () {
        NETDATA.setOption('concurrent_refreshes', $(this).prop('checked'));
    });
    $('#sync_selection').change(function () {
        NETDATA.setOption('sync_selection', $(this).prop('checked'));
    });
    $('#sync_pan_and_zoom').change(function () {
        NETDATA.setOption('sync_pan_and_zoom', $(this).prop('checked'));
    });
    $('#stop_updates_when_focus_is_lost').change(function () {
        urlOptions.update_always = !$(this).prop('checked');
        urlOptions.hashUpdate();

        NETDATA.setOption('stop_updates_when_focus_is_lost', !urlOptions.update_always);
    });
    $('#smooth_plot').change(function () {
        NETDATA.setOption('smooth_plot', $(this).prop('checked'));
    });
    $('#pan_and_zoom_data_padding').change(function () {
        NETDATA.setOption('pan_and_zoom_data_padding', $(this).prop('checked'));
    });
    $('#seconds_as_time').change(function () {
        NETDATA.setOption('seconds_as_time', $(this).prop('checked'));
    });
    $('#local_timezone').change(function () {
        if ($(this).prop('checked')) {
            selected_server_timezone('default', true);
        } else {
            selected_server_timezone('default', false);
        }
    });

    $('#units_conversion').change(function () {
        NETDATA.setOption('units', $(this).prop('checked') ? 'auto' : 'original');
    });
    $('#units_temp').change(function () {
        NETDATA.setOption('temperature', $(this).prop('checked') ? 'celsius' : 'fahrenheit');
    });

    $('#show_help').change(function () {
        urlOptions.help = $(this).prop('checked');
        urlOptions.hashUpdate();

        NETDATA.setOption('show_help', urlOptions.help);
        netdataReload();
    });

    // this has to be the last
    // it reloads the page
    $('#netdata_theme_control').change(function () {
        urlOptions.theme = $(this).prop('checked') ? 'slate' : 'white';
        urlOptions.hashUpdate();

        if (setTheme(urlOptions.theme)) {
            netdataReload();
        }
    });
}

function scrollDashboardTo() {
    if (netdataSnapshotData !== null && typeof netdataSnapshotData.hash !== 'undefined') {
        //console.log(netdataSnapshotData.hash);
        scrollToId(netdataSnapshotData.hash.replace('#', ''));
    } else {
        // check if we have to jump to a specific section
        scrollToId(urlOptions.hash.replace('#', ''));

        if (urlOptions.chart !== null) {
            NETDATA.alarms.scrollToChart(urlOptions.chart);
            //urlOptions.hash = '#' + NETDATA.name2id('menu_' + charts[c].menu + '_submenu_' + charts[c].submenu);
            //urlOptions.hash = '#chart_' + NETDATA.name2id(urlOptions.chart);
            //console.log('hash = ' + urlOptions.hash);
        }
    }
}

var modalHiddenCallback = null;

function scrollToChartAfterHidingModal(chart) {
    modalHiddenCallback = function () {
        NETDATA.alarms.scrollToChart(chart);
    };
}

// ----------------------------------------------------------------------------

function enableTooltipsAndPopovers() {
    $('[data-toggle="tooltip"]').tooltip({
        animated: 'fade',
        trigger: 'hover',
        html: true,
        delay: {show: 500, hide: 0},
        container: 'body'
    });
    $('[data-toggle="popover"]').popover();
}

// ----------------------------------------------------------------------------

var runOnceOnDashboardLastRun = 0;

function runOnceOnDashboardWithjQuery() {
    if (runOnceOnDashboardLastRun !== 0) {
        scrollDashboardTo();

        // restore the scrollspy at the proper position
        $(document.body).scrollspy('refresh');
        $(document.body).scrollspy('process');

        return;
    }

    runOnceOnDashboardLastRun = Date.now();

    // ------------------------------------------------------------------------
    // bootstrap modals

    // prevent bootstrap modals from scrolling the page
    // maintains the current scroll position
    // https://stackoverflow.com/a/34754029/4525767

    var scrollPos = 0;
    var modal_depth = 0;                            // how many modals are currently open
    var modal_shown = false;                        // set to true, if a modal is shown
    var netdata_paused_on_modal = false;            // set to true, if the modal paused netdata
    var scrollspyOffset = $(window).height() / 3;   // will be updated below - the offset of scrollspy to select an item

    $('.modal')
        .on('show.bs.modal', function () {
            if (modal_depth === 0) {
                scrollPos = window.scrollY;

                $('body').css({
                    overflow: 'hidden',
                    position: 'fixed',
                    top: -scrollPos
                });

                modal_shown = true;

                if (NETDATA.options.pauseCallback === null) {
                    NETDATA.pause(function () {
                    });
                    netdata_paused_on_modal = true;
                } else {
                    netdata_paused_on_modal = false;
                }
            }

            modal_depth++;
            //console.log(urlOptions.after);

        })
        .on('hide.bs.modal', function () {

            modal_depth--;

            if (modal_depth <= 0) {
                modal_depth = 0;

                $('body')
                    .css({
                        overflow: '',
                        position: '',
                        top: ''
                    });

                // scroll to the position we had open before the modal
                $('html, body')
                    .animate({scrollTop: scrollPos}, 0);

                // unpause netdata, if we paused it
                if (netdata_paused_on_modal === true) {
                    NETDATA.unpause();
                    netdata_paused_on_modal = false;
                }

                // restore the scrollspy at the proper position
                $(document.body).scrollspy('process');
            }
            //console.log(urlOptions.after);
        })
        .on('hidden.bs.modal', function () {
            if (modal_depth === 0) {
                modal_shown = false;
            }

            if (typeof modalHiddenCallback === 'function') {
                modalHiddenCallback();
            }

            modalHiddenCallback = null;
            //console.log(urlOptions.after);
        });

    // ------------------------------------------------------------------------
    // sidebar / affix

    $('#sidebar')
        .affix({
            offset: {
                top: (isdemo()) ? 150 : 0,
                bottom: 0
            }
        })
        .on('affixed.bs.affix', function () {
            // fix scrolling of very long affix lists
            // http://stackoverflow.com/questions/21691585/bootstrap-3-1-0-affix-too-long

            $(this).removeAttr('style');
        })
        .on('affix-top.bs.affix', function () {
            // fix bootstrap affix click bug
            // https://stackoverflow.com/a/37847981/4525767

            if (modal_shown) {
                return false;
            }
        })
        .on('activate.bs.scrollspy', function (e) {
            // change the URL based on the current position of the screen

            if (modal_shown === false) {
                var el = $(e.target);
                var hash = el.find('a').attr('href');
                if (typeof hash === 'string' && hash.substring(0, 1) === '#' && urlOptions.hash.startsWith(hash + '_submenu_') === false) {
                    urlOptions.hash = hash;
                    urlOptions.hashUpdate();
                }
            }
        });

    Ps.initialize(document.getElementById('sidebar'), {
        wheelSpeed: 0.5,
        wheelPropagation: true,
        swipePropagation: true,
        minScrollbarLength: null,
        maxScrollbarLength: null,
        useBothWheelAxes: false,
        suppressScrollX: true,
        suppressScrollY: false,
        scrollXMarginOffset: 0,
        scrollYMarginOffset: 0,
        theme: 'default'
    });

    // ------------------------------------------------------------------------
    // scrollspy

    if (scrollspyOffset > 250) {
        scrollspyOffset = 250;
    }
    if (scrollspyOffset < 75) {
        scrollspyOffset = 75;
    }
    document.body.setAttribute('data-offset', scrollspyOffset);

    // scroll the dashboard, before activating the scrollspy, so that our
    // hash will not be updated before we got the chance to scroll to it
    scrollDashboardTo();

    $(document.body).scrollspy({
        target: '#sidebar',
        offset: scrollspyOffset // controls the diff of the <hX> element to the top, to select it
    });

    // ------------------------------------------------------------------------
    // my-netdata menu

    Ps.initialize(document.getElementById('my-netdata-dropdown-content'), {
        wheelSpeed: 1,
        wheelPropagation: false,
        swipePropagation: false,
        minScrollbarLength: null,
        maxScrollbarLength: null,
        useBothWheelAxes: false,
        suppressScrollX: true,
        suppressScrollY: false,
        scrollXMarginOffset: 0,
        scrollYMarginOffset: 0,
        theme: 'default'
    });

    $('#myNetdataDropdownParent')
        .on('show.bs.dropdown', function () {
            var hash = urlOptions.genHash();
            $('.registry_link').each(function (idx) {
                this.setAttribute('href', this.getAttribute("href").replace(/#.*$/, hash));
            });

            NETDATA.pause(function () {
            });
        })
        .on('shown.bs.dropdown', function () {
            Ps.update(document.getElementById('my-netdata-dropdown-content'));
        })
        .on('hidden.bs.dropdown', function () {
            NETDATA.unpause();
        });

    $('#deleteRegistryModal')
        .on('hidden.bs.modal', function () {
            deleteRegistryGuid = null;
        });

    // ------------------------------------------------------------------------
    // update modal

    $('#updateModal')
        .on('show.bs.modal', function () {
            versionLog('checking, please wait...');
        })
        .on('shown.bs.modal', function () {
            notifyForUpdate(true);
        });

    // ------------------------------------------------------------------------
    // alarms modal

    $('#alarmsModal')
        .on('shown.bs.modal', function () {
            alarmsUpdateModal();
        })
        .on('hidden.bs.modal', function () {
            document.getElementById('alarms_active').innerHTML =
                document.getElementById('alarms_all').innerHTML =
                    document.getElementById('alarms_log').innerHTML =
                        'loading...';
        });

    // ------------------------------------------------------------------------

    dashboardSettingsSetup();
    loadSnapshotDragAndDropSetup();
    saveSnapshotModalSetup();
    showPageFooter();

    // ------------------------------------------------------------------------
    // https://github.com/viralpatel/jquery.shorten/blob/master/src/jquery.shorten.js

    $.fn.shorten = function (settings) {
        "use strict";

        var config = {
            showChars: 750,
            minHideChars: 10,
            ellipsesText: "...",
            moreText: '<i class="fas fa-expand"></i> show more information',
            lessText: '<i class="fas fa-compress"></i> show less information',
            onLess: function () {
                NETDATA.onscroll();
            },
            onMore: function () {
                NETDATA.onscroll();
            },
            errMsg: null,
            force: false
        };

        if (settings) {
            $.extend(config, settings);
        }

        if ($(this).data('jquery.shorten') && !config.force) {
            return false;
        }
        $(this).data('jquery.shorten', true);

        $(document).off("click", '.morelink');

        $(document).on({
            click: function () {

                var $this = $(this);
                if ($this.hasClass('less')) {
                    $this.removeClass('less');
                    $this.html(config.moreText);
                    $this.parent().prev().animate({'height': '0' + '%'}, 0, function () {
                        $this.parent().prev().prev().show();
                    }).hide(0, function () {
                        config.onLess();
                    });
                } else {
                    $this.addClass('less');
                    $this.html(config.lessText);
                    $this.parent().prev().animate({'height': '100' + '%'}, 0, function () {
                        $this.parent().prev().prev().hide();
                    }).show(0, function () {
                        config.onMore();
                    });
                }
                return false;
            }
        }, '.morelink');

        return this.each(function () {
            var $this = $(this);

            var content = $this.html();
            var contentlen = $this.text().length;
            if (contentlen > config.showChars + config.minHideChars) {
                var c = content.substr(0, config.showChars);
                if (c.indexOf('<') >= 0) // If there's HTML don't want to cut it
                {
                    var inTag = false; // I'm in a tag?
                    var bag = ''; // Put the characters to be shown here
                    var countChars = 0; // Current bag size
                    var openTags = []; // Stack for opened tags, so I can close them later
                    var tagName = null;

                    for (var i = 0, r = 0; r <= config.showChars; i++) {
                        if (content[i] === '<' && !inTag) {
                            inTag = true;

                            // This could be "tag" or "/tag"
                            tagName = content.substring(i + 1, content.indexOf('>', i));

                            // If its a closing tag
                            if (tagName[0] === '/') {

                                if (tagName !== ('/' + openTags[0])) {
                                    config.errMsg = 'ERROR en HTML: the top of the stack should be the tag that closes';
                                } else {
                                    openTags.shift(); // Pops the last tag from the open tag stack (the tag is closed in the retult HTML!)
                                }

                            } else {
                                // There are some nasty tags that don't have a close tag like <br/>
                                if (tagName.toLowerCase() !== 'br') {
                                    openTags.unshift(tagName); // Add to start the name of the tag that opens
                                }
                            }
                        }
                        
                        if (inTag && content[i] === '>') {
                            inTag = false;
                        }

                        if (inTag) {
                            bag += content.charAt(i);
                        } else {
                            // Add tag name chars to the result
                            r++;
                            if (countChars <= config.showChars) {
                                bag += content.charAt(i); // Fix to ie 7 not allowing you to reference string characters using the []
                                countChars++;
                            } else {
                                // Now I have the characters needed
                                if (openTags.length > 0) {
                                    // I have unclosed tags

                                    //console.log('They were open tags');
                                    //console.log(openTags);
                                    for (var j = 0; j < openTags.length; j++) {
                                        //console.log('Cierro tag ' + openTags[j]);
                                        bag += '</' + openTags[j] + '>'; // Close all tags that were opened

                                        // You could shift the tag from the stack to check if you end with an empty stack, that means you have closed all open tags
                                    }
                                    break;
                                }
                            }
                        }
                    }
                    c = $('<div/>').html(bag + '<span class="ellip">' + config.ellipsesText + '</span>').html();
                } else {
                    c += config.ellipsesText;
                }

                var html = '<div class="shortcontent">' + c +
                    '</div><div class="allcontent">' + content +
                    '</div><span><a href="javascript://nop/" class="morelink">' + config.moreText + '</a></span>';

                $this.html(html);
                $this.find(".allcontent").hide(); // Hide all text
                $('.shortcontent p:last', $this).css('margin-bottom', 0); //Remove bottom margin on last paragraph as it's likely shortened
            }
        });
    };
}

function finalizePage() {
    // resize all charts - without starting the background thread
    // this has to be done while NETDATA is paused
    // if we ommit this, the affix menu will be wrong, since all
    // the Dom elements are initially zero-sized
    NETDATA.parseDom();

    // ------------------------------------------------------------------------

    NETDATA.globalPanAndZoom.callback = null;
    NETDATA.globalChartUnderlay.callback = null;

    if (urlOptions.pan_and_zoom === true && NETDATA.options.targets.length > 0) {
        NETDATA.globalPanAndZoom.setMaster(NETDATA.options.targets[0], urlOptions.after, urlOptions.before);
    }

    // callback for us to track PanAndZoom operations
    NETDATA.globalPanAndZoom.callback = urlOptions.netdataPanAndZoomCallback;
    NETDATA.globalChartUnderlay.callback = urlOptions.netdataHighlightCallback;

    // ------------------------------------------------------------------------

    // let it run (update the charts)
    NETDATA.unpause();

    runOnceOnDashboardWithjQuery();
    $(".shorten").shorten();
    enableTooltipsAndPopovers();

    if (isdemo()) {
        // do not to give errors on netdata demo servers for 60 seconds
        NETDATA.options.current.retries_on_data_failures = 60;

        if (urlOptions.nowelcome !== true) {
            setTimeout(function () {
                $('#welcomeModal').modal();
            }, 1000);
        }

        // google analytics when this is used for the home page of the demo sites
        // this does not run on user's installations
        setTimeout(function () {
            (function (i, s, o, g, r, a, m) {
                i['GoogleAnalyticsObject'] = r;
                i[r] = i[r] || function () {
                    (i[r].q = i[r].q || []).push(arguments)
                }, i[r].l = 1 * new Date();
                a = s.createElement(o),
                    m = s.getElementsByTagName(o)[0];
                a.async = 1;
                a.src = g;
                m.parentNode.insertBefore(a, m)
            })(window, document, 'script', 'https://www.google-analytics.com/analytics.js', 'ga');

            ga('create', 'UA-64295674-3', 'auto');
            ga('send', 'pageview');
        }, 2000);
    } else {
        notifyForUpdate();
    }

    if (urlOptions.show_alarms === true) {
        setTimeout(function () {
            $('#alarmsModal').modal('show');
        }, 1000);
    }

    NETDATA.onresizeCallback = function () {
        Ps.update(document.getElementById('sidebar'));
        Ps.update(document.getElementById('my-netdata-dropdown-content'));
    };
    NETDATA.onresizeCallback();

    if (netdataSnapshotData !== null) {
        NETDATA.globalPanAndZoom.setMaster(NETDATA.options.targets[0], netdataSnapshotData.after_ms, netdataSnapshotData.before_ms);
    }

    // var netdataEnded = performance.now();
    // console.log('start up time: ' + (netdataEnded - netdataStarted).toString() + ' ms');
}

function resetDashboardOptions() {
    var help = NETDATA.options.current.show_help;

    NETDATA.resetOptions();
    if (setTheme('slate')) {
        netdataReload();
    }

    if (help !== NETDATA.options.current.show_help) {
        netdataReload();
    }
}

// callback to add the dashboard info to the
// parallel javascript downloader in netdata
var netdataPrepCallback = function () {
    NETDATA.requiredCSS.push({
        url: NETDATA.serverStatic + 'css/bootstrap-toggle-2.2.2.min.css',
        isAlreadyLoaded: function () {
            return false;
        }
    });

    NETDATA.requiredJs.push({
        url: NETDATA.serverStatic + 'lib/bootstrap-toggle-2.2.2.min.js',
        isAlreadyLoaded: function () {
            return false;
        }
    });

    NETDATA.requiredJs.push({
        url: NETDATA.serverStatic + 'dashboard_info.js?v20181019-1',
        async: false,
        isAlreadyLoaded: function () {
            return false;
        }
    });

    if (isdemo()) {
        document.getElementById('masthead').style.display = 'block';
    } else {
        if (urlOptions.update_always === true) {
            NETDATA.setOption('stop_updates_when_focus_is_lost', !urlOptions.update_always);
        }
    }
};

var selected_server_timezone = function (timezone, status) {
    //console.log('called with timezone: ' + timezone + ", status: " + ((typeof status === 'undefined')?'undefined':status).toString());

    // clear the error
    document.getElementById('timezone_error_message').innerHTML = '';

    if (typeof status === 'undefined') {
        // the user selected a timezone from the menu

        NETDATA.setOption('user_set_server_timezone', timezone);

        if (NETDATA.dateTime.init(timezone) === false) {
            NETDATA.dateTime.init();

            if (!$('#local_timezone').prop('checked')) {
                $('#local_timezone').bootstrapToggle('on');
            }

            document.getElementById('timezone_error_message').innerHTML = 'Ooops! That timezone was not accepted by your browser. Please open a github issue to help us fix it.';
            NETDATA.setOption('user_set_server_timezone', NETDATA.options.server_timezone);
        } else {
            if ($('#local_timezone').prop('checked')) {
                $('#local_timezone').bootstrapToggle('off');
            }
        }
    } else if (status === true) {
        // the user wants the browser default timezone to be activated

        NETDATA.dateTime.init();
    } else {
        // the user wants the server default timezone to be activated
        //console.log('found ' + NETDATA.options.current.user_set_server_timezone);

        if (NETDATA.options.current.user_set_server_timezone === 'default') {
            NETDATA.options.current.user_set_server_timezone = NETDATA.options.server_timezone;
        }

        timezone = NETDATA.options.current.user_set_server_timezone;

        if (NETDATA.dateTime.init(timezone) === false) {
            NETDATA.dateTime.init();

            if (!$('#local_timezone').prop('checked')) {
                $('#local_timezone').bootstrapToggle('on');
            }

            document.getElementById('timezone_error_message').innerHTML = 'Sorry. The timezone "' + timezone.toString() + '" is not accepted by your browser. Please select one from the list.';
            NETDATA.setOption('user_set_server_timezone', NETDATA.options.server_timezone);
        }
    }

    document.getElementById('current_timezone').innerText = (NETDATA.options.current.timezone === 'default') ? 'unset, using browser default' : NETDATA.options.current.timezone;
    return false;
};

// our entry point
// var netdataStarted = performance.now();

var netdataCallback = initializeDynamicDashboard;
