
// Charts Libraries Registration

NETDATA.chartLibraries = {
    "dygraph": {
        initialize: NETDATA.dygraphInitialize,
        create: NETDATA.dygraphChartCreate,
        update: NETDATA.dygraphChartUpdate,
        resize: function (state) {
            if (typeof state.tmp.dygraph_instance !== 'undefined' && typeof state.tmp.dygraph_instance.resize === 'function') {
                state.tmp.dygraph_instance.resize();
            }
        },
        setSelection: NETDATA.dygraphSetSelection,
        clearSelection: NETDATA.dygraphClearSelection,
        toolboxPanAndZoom: NETDATA.dygraphToolboxPanAndZoom,
        initialized: false,
        enabled: true,
        xssRegexIgnore: new RegExp('^/api/v1/data\.result.data$'),
        format: function (state) {
            void(state);
            return 'json';
        },
        options: function (state) {
            return 'ms' + '%7C' + 'flip' + (this.isLogScale(state) ? ('%7C' + 'abs') : '').toString();
        },
        legend: function (state) {
            return (this.isSparkline(state) === false && NETDATA.dataAttributeBoolean(state.element, 'legend', true) === true) ? 'right-side' : null;
        },
        autoresize: function (state) {
            void(state);
            return true;
        },
        max_updates_to_recreate: function (state) {
            void(state);
            return 5000;
        },
        track_colors: function (state) {
            void(state);
            return true;
        },
        pixels_per_point: function (state) {
            return (this.isSparkline(state) === false) ? 3 : 2;
        },
        isSparkline: function (state) {
            if (typeof state.tmp.dygraph_sparkline === 'undefined') {
                state.tmp.dygraph_sparkline = (this.theme(state) === 'sparkline');
            }
            return state.tmp.dygraph_sparkline;
        },
        isLogScale: function (state) {
            if (typeof state.tmp.dygraph_logscale === 'undefined') {
                state.tmp.dygraph_logscale = (this.theme(state) === 'logscale');
            }
            return state.tmp.dygraph_logscale;
        },
        theme: function (state) {
            if (typeof state.tmp.dygraph_theme === 'undefined') {
                state.tmp.dygraph_theme = NETDATA.dataAttribute(state.element, 'dygraph-theme', 'default');
            }
            return state.tmp.dygraph_theme;
        },
        container_class: function (state) {
            if (this.legend(state) !== null) {
                return 'netdata-container-with-legend';
            }
            return 'netdata-container';
        }
    },
    "sparkline": {
        initialize: NETDATA.sparklineInitialize,
        create: NETDATA.sparklineChartCreate,
        update: NETDATA.sparklineChartUpdate,
        resize: null,
        setSelection: undefined, // function(state, t) { void(state); return true; },
        clearSelection: undefined, // function(state) { void(state); return true; },
        toolboxPanAndZoom: null,
        initialized: false,
        enabled: true,
        xssRegexIgnore: new RegExp('^/api/v1/data\.result$'),
        format: function (state) {
            void(state);
            return 'array';
        },
        options: function (state) {
            void(state);
            return 'flip' + '%7C' + 'abs';
        },
        legend: function (state) {
            void(state);
            return null;
        },
        autoresize: function (state) {
            void(state);
            return false;
        },
        max_updates_to_recreate: function (state) {
            void(state);
            return 5000;
        },
        track_colors: function (state) {
            void(state);
            return false;
        },
        pixels_per_point: function (state) {
            void(state);
            return 3;
        },
        container_class: function (state) {
            void(state);
            return 'netdata-container';
        }
    },
    "peity": {
        initialize: NETDATA.peityInitialize,
        create: NETDATA.peityChartCreate,
        update: NETDATA.peityChartUpdate,
        resize: null,
        setSelection: undefined, // function(state, t) { void(state); return true; },
        clearSelection: undefined, // function(state) { void(state); return true; },
        toolboxPanAndZoom: null,
        initialized: false,
        enabled: true,
        xssRegexIgnore: new RegExp('^/api/v1/data\.result$'),
        format: function (state) {
            void(state);
            return 'ssvcomma';
        },
        options: function (state) {
            void(state);
            return 'null2zero' + '%7C' + 'flip' + '%7C' + 'abs';
        },
        legend: function (state) {
            void(state);
            return null;
        },
        autoresize: function (state) {
            void(state);
            return false;
        },
        max_updates_to_recreate: function (state) {
            void(state);
            return 5000;
        },
        track_colors: function (state) {
            void(state);
            return false;
        },
        pixels_per_point: function (state) {
            void(state);
            return 3;
        },
        container_class: function (state) {
            void(state);
            return 'netdata-container';
        }
    },
    // "morris": {
    //     initialize: NETDATA.morrisInitialize,
    //     create: NETDATA.morrisChartCreate,
    //     update: NETDATA.morrisChartUpdate,
    //     resize: null,
    //     setSelection: undefined, // function(state, t) { void(state); return true; },
    //     clearSelection: undefined, // function(state) { void(state); return true; },
    //     toolboxPanAndZoom: null,
    //     initialized: false,
    //     enabled: true,
    //     xssRegexIgnore: new RegExp('^/api/v1/data\.result.data$'),
    //     format: function(state) { void(state); return 'json'; },
    //     options: function(state) { void(state); return 'objectrows' + '%7C' + 'ms'; },
    //     legend: function(state) { void(state); return null; },
    //     autoresize: function(state) { void(state); return false; },
    //     max_updates_to_recreate: function(state) { void(state); return 50; },
    //     track_colors: function(state) { void(state); return false; },
    //     pixels_per_point: function(state) { void(state); return 15; },
    //     container_class: function(state) { void(state); return 'netdata-container'; }
    // },
    "google": {
        initialize: NETDATA.googleInitialize,
        create: NETDATA.googleChartCreate,
        update: NETDATA.googleChartUpdate,
        resize: null,
        setSelection: undefined, //function(state, t) { void(state); return true; },
        clearSelection: undefined, //function(state) { void(state); return true; },
        toolboxPanAndZoom: null,
        initialized: false,
        enabled: true,
        xssRegexIgnore: new RegExp('^/api/v1/data\.result.rows$'),
        format: function (state) {
            void(state);
            return 'datatable';
        },
        options: function (state) {
            void(state);
            return '';
        },
        legend: function (state) {
            void(state);
            return null;
        },
        autoresize: function (state) {
            void(state);
            return false;
        },
        max_updates_to_recreate: function (state) {
            void(state);
            return 300;
        },
        track_colors: function (state) {
            void(state);
            return false;
        },
        pixels_per_point: function (state) {
            void(state);
            return 4;
        },
        container_class: function (state) {
            void(state);
            return 'netdata-container';
        }
    },
    // "raphael": {
    //     initialize: NETDATA.raphaelInitialize,
    //     create: NETDATA.raphaelChartCreate,
    //     update: NETDATA.raphaelChartUpdate,
    //     resize: null,
    //     setSelection: undefined, // function(state, t) { void(state); return true; },
    //     clearSelection: undefined, // function(state) { void(state); return true; },
    //     toolboxPanAndZoom: null,
    //     initialized: false,
    //     enabled: true,
    //     xssRegexIgnore: new RegExp('^/api/v1/data\.result.data$'),
    //     format: function(state) { void(state); return 'json'; },
    //     options: function(state) { void(state); return ''; },
    //     legend: function(state) { void(state); return null; },
    //     autoresize: function(state) { void(state); return false; },
    //     max_updates_to_recreate: function(state) { void(state); return 5000; },
    //     track_colors: function(state) { void(state); return false; },
    //     pixels_per_point: function(state) { void(state); return 3; },
    //     container_class: function(state) { void(state); return 'netdata-container'; }
    // },
    // "c3": {
    //     initialize: NETDATA.c3Initialize,
    //     create: NETDATA.c3ChartCreate,
    //     update: NETDATA.c3ChartUpdate,
    //     resize: null,
    //     setSelection: undefined, // function(state, t) { void(state); return true; },
    //     clearSelection: undefined, // function(state) { void(state); return true; },
    //     toolboxPanAndZoom: null,
    //     initialized: false,
    //     enabled: true,
    //     xssRegexIgnore: new RegExp('^/api/v1/data\.result$'),
    //     format: function(state) { void(state); return 'csvjsonarray'; },
    //     options: function(state) { void(state); return 'milliseconds'; },
    //     legend: function(state) { void(state); return null; },
    //     autoresize: function(state) { void(state); return false; },
    //     max_updates_to_recreate: function(state) { void(state); return 5000; },
    //     track_colors: function(state) { void(state); return false; },
    //     pixels_per_point: function(state) { void(state); return 15; },
    //     container_class: function(state) { void(state); return 'netdata-container'; }
    // },
    "d3pie": {
        initialize: NETDATA.d3pieInitialize,
        create: NETDATA.d3pieChartCreate,
        update: NETDATA.d3pieChartUpdate,
        resize: null,
        setSelection: NETDATA.d3pieSetSelection,
        clearSelection: NETDATA.d3pieClearSelection,
        toolboxPanAndZoom: null,
        initialized: false,
        enabled: true,
        xssRegexIgnore: new RegExp('^/api/v1/data\.result.data$'),
        format: function (state) {
            void(state);
            return 'json';
        },
        options: function (state) {
            void(state);
            return 'objectrows' + '%7C' + 'ms';
        },
        legend: function (state) {
            void(state);
            return null;
        },
        autoresize: function (state) {
            void(state);
            return false;
        },
        max_updates_to_recreate: function (state) {
            void(state);
            return 5000;
        },
        track_colors: function (state) {
            void(state);
            return false;
        },
        pixels_per_point: function (state) {
            void(state);
            return 15;
        },
        container_class: function (state) {
            void(state);
            return 'netdata-container';
        }
    },
    "d3": {
        initialize: NETDATA.d3Initialize,
        create: NETDATA.d3ChartCreate,
        update: NETDATA.d3ChartUpdate,
        resize: null,
        setSelection: undefined, // function(state, t) { void(state); return true; },
        clearSelection: undefined, // function(state) { void(state); return true; },
        toolboxPanAndZoom: null,
        initialized: false,
        enabled: true,
        xssRegexIgnore: new RegExp('^/api/v1/data\.result.data$'),
        format: function (state) {
            void(state);
            return 'json';
        },
        options: function (state) {
            void(state);
            return '';
        },
        legend: function (state) {
            void(state);
            return null;
        },
        autoresize: function (state) {
            void(state);
            return false;
        },
        max_updates_to_recreate: function (state) {
            void(state);
            return 5000;
        },
        track_colors: function (state) {
            void(state);
            return false;
        },
        pixels_per_point: function (state) {
            void(state);
            return 3;
        },
        container_class: function (state) {
            void(state);
            return 'netdata-container';
        }
    },
    "easypiechart": {
        initialize: NETDATA.easypiechartInitialize,
        create: NETDATA.easypiechartChartCreate,
        update: NETDATA.easypiechartChartUpdate,
        resize: null,
        setSelection: NETDATA.easypiechartSetSelection,
        clearSelection: NETDATA.easypiechartClearSelection,
        toolboxPanAndZoom: null,
        initialized: false,
        enabled: true,
        xssRegexIgnore: new RegExp('^/api/v1/data\.result$'),
        format: function (state) {
            void(state);
            return 'array';
        },
        options: function (state) {
            void(state);
            return 'absolute';
        },
        legend: function (state) {
            void(state);
            return null;
        },
        autoresize: function (state) {
            void(state);
            return false;
        },
        max_updates_to_recreate: function (state) {
            void(state);
            return 5000;
        },
        track_colors: function (state) {
            void(state);
            return true;
        },
        pixels_per_point: function (state) {
            void(state);
            return 3;
        },
        aspect_ratio: 100,
        container_class: function (state) {
            void(state);
            return 'netdata-container-easypiechart';
        }
    },
    "gauge": {
        initialize: NETDATA.gaugeInitialize,
        create: NETDATA.gaugeChartCreate,
        update: NETDATA.gaugeChartUpdate,
        resize: null,
        setSelection: NETDATA.gaugeSetSelection,
        clearSelection: NETDATA.gaugeClearSelection,
        toolboxPanAndZoom: null,
        initialized: false,
        enabled: true,
        xssRegexIgnore: new RegExp('^/api/v1/data\.result$'),
        format: function (state) {
            void(state);
            return 'array';
        },
        options: function (state) {
            void(state);
            return 'absolute';
        },
        legend: function (state) {
            void(state);
            return null;
        },
        autoresize: function (state) {
            void(state);
            return false;
        },
        max_updates_to_recreate: function (state) {
            void(state);
            return 5000;
        },
        track_colors: function (state) {
            void(state);
            return true;
        },
        pixels_per_point: function (state) {
            void(state);
            return 3;
        },
        aspect_ratio: 60,
        container_class: function (state) {
            void(state);
            return 'netdata-container-gauge';
        }
    }
};

NETDATA.registerChartLibrary = function (library, url) {
    if (NETDATA.options.debug.libraries) {
        console.log("registering chart library: " + library);
    }

    NETDATA.chartLibraries[library].url = url;
    NETDATA.chartLibraries[library].initialized = true;
    NETDATA.chartLibraries[library].enabled = true;
};
