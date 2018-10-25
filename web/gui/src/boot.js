
// Boot it!

if (typeof netdataPrepCallback === 'function') {
    netdataPrepCallback();
}

NETDATA.errorReset();
NETDATA.loadRequiredCSS(0);

NETDATA._loadjQuery(function () {
    NETDATA.loadRequiredJs(0, function () {
        if (typeof $().emulateTransitionEnd !== 'function') {
            // bootstrap is not available
            NETDATA.options.current.show_help = false;
        }

        if (typeof netdataDontStart === 'undefined' || !netdataDontStart) {
            if (NETDATA.options.debug.main_loop) {
                console.log('starting chart refresh thread');
            }

            NETDATA.start();
        }
    });
});
