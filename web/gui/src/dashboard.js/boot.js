
// Load required JS libraries and CSS

NETDATA.requiredJs = [
    {
        url: NETDATA.serverStatic + 'lib/bootstrap-3.3.7.min.js',
        async: false,
        isAlreadyLoaded: function () {
            // check if bootstrap is loaded
            if (typeof $().emulateTransitionEnd === 'function') {
                return true;
            } else {
                return typeof netdataNoBootstrap !== 'undefined' && netdataNoBootstrap;
            }
        }
    },
    {
        url: NETDATA.serverStatic + 'lib/fontawesome-all-5.0.1.min.js',
        async: true,
        isAlreadyLoaded: function () {
            return typeof netdataNoFontAwesome !== 'undefined' && netdataNoFontAwesome;
        }
    },
    {
        url: NETDATA.serverStatic + 'lib/perfect-scrollbar-0.6.15.min.js',
        isAlreadyLoaded: function () {
            return false;
        }
    }
];

NETDATA.requiredCSS = [
    {
        url: NETDATA.themes.current.bootstrap_css,
        isAlreadyLoaded: function () {
            return typeof netdataNoBootstrap !== 'undefined' && netdataNoBootstrap;
        }
    },
    {
        url: NETDATA.themes.current.dashboard_css,
        isAlreadyLoaded: function () {
            return false;
        }
    }
];

NETDATA.loadedRequiredJs = 0;
NETDATA.loadRequiredJs = function (index, callback) {
    if (index >= NETDATA.requiredJs.length) {
        if (typeof callback === 'function') {
            return callback();
        }
        return;
    }

    if (NETDATA.requiredJs[index].isAlreadyLoaded()) {
        NETDATA.loadedRequiredJs++;
        NETDATA.loadRequiredJs(++index, callback);
        return;
    }

    if (NETDATA.options.debug.main_loop) {
        console.log('loading ' + NETDATA.requiredJs[index].url);
    }

    let async = true;
    if (typeof NETDATA.requiredJs[index].async !== 'undefined' && NETDATA.requiredJs[index].async === false) {
        async = false;
    }

    $.ajax({
        url: NETDATA.requiredJs[index].url,
        cache: true,
        dataType: "script",
        xhrFields: {withCredentials: true} // required for the cookie
    })
        .done(function () {
            if (NETDATA.options.debug.main_loop) {
                console.log('loaded ' + NETDATA.requiredJs[index].url);
            }
        })
        .fail(function () {
            alert('Cannot load required JS library: ' + NETDATA.requiredJs[index].url);
        })
        .always(function () {
            NETDATA.loadedRequiredJs++;

            // if (async === false)
            if (!async) {
                NETDATA.loadRequiredJs(++index, callback);
            }
        });

    // if (async === true)
    if (async) {
        NETDATA.loadRequiredJs(++index, callback);
    }
};

NETDATA.loadRequiredCSS = function (index) {
    if (index >= NETDATA.requiredCSS.length) {
        return;
    }

    if (NETDATA.requiredCSS[index].isAlreadyLoaded()) {
        NETDATA.loadRequiredCSS(++index);
        return;
    }

    if (NETDATA.options.debug.main_loop) {
        console.log('loading ' + NETDATA.requiredCSS[index].url);
    }

    NETDATA._loadCSS(NETDATA.requiredCSS[index].url);
    NETDATA.loadRequiredCSS(++index);
};

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
