
// *** src/dashboard.js/server-detection.js

if (typeof netdataServer !== 'undefined') {
    NETDATA.serverDefault = netdataServer;
} else {
    let s = NETDATA._scriptSource();
    if (s) {
        NETDATA.serverDefault = s.replace(/\/dashboard.js(\?.*)?$/g, "");
    } else {
        console.log('WARNING: Cannot detect the URL of the netdata server.');
        NETDATA.serverDefault = null;
    }
}

if (NETDATA.serverDefault === null) {
    NETDATA.serverDefault = '';
} else if (NETDATA.serverDefault.slice(-1) !== '/') {
    NETDATA.serverDefault += '/';
}

if (typeof netdataServerStatic !== 'undefined' && netdataServerStatic !== null && netdataServerStatic !== '') {
    NETDATA.serverStatic = netdataServerStatic;
    if (NETDATA.serverStatic.slice(-1) !== '/') {
        NETDATA.serverStatic += '/';
    }
} else {
    NETDATA.serverStatic = NETDATA.serverDefault;
}
