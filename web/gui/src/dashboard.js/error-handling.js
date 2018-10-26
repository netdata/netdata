// Error Handling

NETDATA.errorCodes = {
    100: {message: "Cannot load chart library", alert: true},
    101: {message: "Cannot load jQuery", alert: true},
    402: {message: "Chart library not found", alert: false},
    403: {message: "Chart library not enabled/is failed", alert: false},
    404: {message: "Chart not found", alert: false},
    405: {message: "Cannot download charts index from server", alert: true},
    406: {message: "Invalid charts index downloaded from server", alert: true},
    407: {message: "Cannot HELLO netdata server", alert: false},
    408: {message: "Netdata servers sent invalid response to HELLO", alert: false},
    409: {message: "Cannot ACCESS netdata registry", alert: false},
    410: {message: "Netdata registry ACCESS failed", alert: false},
    411: {message: "Netdata registry server send invalid response to DELETE ", alert: false},
    412: {message: "Netdata registry DELETE failed", alert: false},
    413: {message: "Netdata registry server send invalid response to SWITCH ", alert: false},
    414: {message: "Netdata registry SWITCH failed", alert: false},
    415: {message: "Netdata alarms download failed", alert: false},
    416: {message: "Netdata alarms log download failed", alert: false},
    417: {message: "Netdata registry server send invalid response to SEARCH ", alert: false},
    418: {message: "Netdata registry SEARCH failed", alert: false}
};

NETDATA.errorLast = {
    code: 0,
    message: "",
    datetime: 0
};

NETDATA.error = function (code, msg) {
    NETDATA.errorLast.code = code;
    NETDATA.errorLast.message = msg;
    NETDATA.errorLast.datetime = Date.now();

    console.log("ERROR " + code + ": " + NETDATA.errorCodes[code].message + ": " + msg);

    let ret = true;
    if (typeof netdataErrorCallback === 'function') {
        ret = netdataErrorCallback('system', code, msg);
    }

    if (ret && NETDATA.errorCodes[code].alert) {
        alert("ERROR " + code + ": " + NETDATA.errorCodes[code].message + ": " + msg);
    }
};

NETDATA.errorReset = function () {
    NETDATA.errorLast.code = 0;
    NETDATA.errorLast.message = "You are doing fine!";
    NETDATA.errorLast.datetime = 0;
};
