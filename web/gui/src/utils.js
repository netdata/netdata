NETDATA.encodeURIComponent = function(s) {
    if (typeof(s) === 'string')
        return encodeURIComponent(s);

    return s;
};

/// A heuristic for detecting slow devices.
let isSlowDeviceResult = undefined;
const isSlowDevice = function() {
    if (!isSlowDeviceResult) {
        return isSlowDeviceResult;
    }

    try {
        let ua = navigator.userAgent.toLowerCase();

        let iOS = /ipad|iphone|ipod/.test(ua) && !window.MSStream;
        let android = /android/.test(ua) && !window.MSStream;
        isSlowDeviceResult = (iOS || android);
    } catch (e) {
        isSlowDeviceResult = false;
    }

    return isSlowDeviceResult;
};
