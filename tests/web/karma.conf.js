// Karma configuration
// Generated on Sun Jul 16 2017 02:28:05 GMT+0200 (CEST)

module.exports = function (config) {
    config.set({

        // base path that will be used to resolve all patterns (eg. files, exclude)
        // this path should always resolve so that "." is the "netdata" root folder.
        basePath: '../../',

        // frameworks to use
        // available frameworks: https://npmjs.org/browse/keyword/karma-adapter
        frameworks: ['jasmine'],


        // list of files / patterns to load in the browser
        files: [
            // order matters! load jquery libraries first
            'web/lib/jquery*.js',
            // our jasmine libs and fixtures
            'tests/web/lib/*.js',
            'tests/web/fixtures/*.html',
            // then bootstrap
            'web/lib/bootstrap*.js',
            // then the rest
            'web/lib/perfect-scrollbar*.js',
            'web/lib/dygraph*.js',
            'web/lib/gauge*.js',
            'web/lib/morris*.js',
            'web/lib/raphael*.js',
            'web/lib/tableExport*.js',
            'web/lib/d3*.js',
            'web/lib/c3*.js',
            // some CSS
            'web/css/*.css',
            'web/dashboard.css',
            // our dashboard
            'web/dashboard.js',
            // finally our test specs
            'tests/web/*.spec.js',
        ],


        // list of files to exclude
        exclude: [],


        // preprocess matching files before serving them to the browser
        // available preprocessors: https://npmjs.org/browse/keyword/karma-preprocessor
        preprocessors: {
            'web/dashboard.js': ['coverage']
        },


        // test results reporter to use
        // possible values: 'dots', 'progress'
        // available reporters: https://npmjs.org/browse/keyword/karma-reporter
        reporters: ['progress', 'coverage'],

        // optionally, configure the reporter
        coverageReporter: {
            type : 'html',
            dir : 'coverage/'
        },

        // web server port
        port: 9876,


        // enable / disable colors in the output (reporters and logs)
        colors: true,


        // level of logging
        // possible values: config.LOG_DISABLE || config.LOG_ERROR || config.LOG_WARN || config.LOG_INFO || config.LOG_DEBUG
        logLevel: config.LOG_INFO,


        // enable / disable watching file and executing tests whenever any file changes
        autoWatch: false,
        // not needed with WebStorm. Just hit Alt+Shift+R to rerun.

        // start these browsers
        // available browser launchers: https://npmjs.org/browse/keyword/karma-launcher
        browsers: ['Chromium', 'ChromiumHeadless'],

        customLaunchers: {
            // Headless browsers could be useful for CI integration, if installed.
            ChromiumHeadless: {
                // needs Chrome/Chromium version >= 59
                // see https://chromium.googlesource.com/chromium/src/+/lkgr/headless/README.md
                base: "Chromium",
                flags: [
                    "--headless",
                    "--disable-gpu",
                    // Without a remote debugging port, Chromium exits immediately.
                    "--remote-debugging-port=9222"
                ]
            }
        },

        // Continuous Integration mode
        // if true, Karma captures browsers, runs the tests and exits
        singleRun: false,

        // Concurrency level
        // how many browser should be started simultaneous
        concurrency: Infinity
    })
};
