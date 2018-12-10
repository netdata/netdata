
NETDATA.icons = {
    left: '<i class="fas fa-backward"></i>',
    reset: '<i class="fas fa-play"></i>',
    right: '<i class="fas fa-forward"></i>',
    zoomIn: '<i class="fas fa-plus"></i>',
    zoomOut: '<i class="fas fa-minus"></i>',
    resize: '<i class="fas fa-sort"></i>',
    lineChart: '<i class="fas fa-chart-line"></i>',
    areaChart: '<i class="fas fa-chart-area"></i>',
    noChart: '<i class="fas fa-chart-area"></i>',
    loading: '<i class="fas fa-sync-alt"></i>',
    noData: '<i class="fas fa-exclamation-triangle"></i>'
};

if (typeof netdataIcons === 'object') {
    // for (let icon in NETDATA.icons) {
    //     if (NETDATA.icons.hasOwnProperty(icon) && typeof(netdataIcons[icon]) === 'string')
    //         NETDATA.icons[icon] = netdataIcons[icon];
    // }
    for (var icon of Object.keys(NETDATA.icons)) {
        if (typeof(netdataIcons[icon]) === 'string') {
            NETDATA.icons[icon] = netdataIcons[icon]
        }
    }
}

if (typeof netdataSnapshotData === 'undefined') {
    netdataSnapshotData = null;
}

if (typeof netdataShowHelp === 'undefined') {
    netdataShowHelp = true;
}

if (typeof netdataShowAlarms === 'undefined') {
    netdataShowAlarms = false;
}

if (typeof netdataRegistryAfterMs !== 'number' || netdataRegistryAfterMs < 0) {
    netdataRegistryAfterMs = 1500;
}

if (typeof netdataRegistry === 'undefined') {
    // backward compatibility
    netdataRegistry = (typeof netdataNoRegistry !== 'undefined' && netdataNoRegistry === false);
}

if (netdataRegistry === false && typeof netdataRegistryCallback === 'function') {
    netdataRegistry = true;
}

// ----------------------------------------------------------------------------------------------------------------
// the defaults for all charts

// if the user does not specify any of these, the following will be used

NETDATA.chartDefaults = {
    width: '100%',                  // the chart width - can be null
    height: '100%',                 // the chart height - can be null
    min_width: null,                // the chart minimum width - can be null
    library: 'dygraph',             // the graphing library to use
    method: 'average',              // the grouping method
    before: 0,                      // panning
    after: -600,                    // panning
    pixels_per_point: 1,            // the detail of the chart
    fill_luminance: 0.8             // luminance of colors in solid areas
};

// ----------------------------------------------------------------------------------------------------------------
// global options

NETDATA.options = {
    pauseCallback: null,            // a callback when we are really paused

    pause: false,                   // when enabled we don't auto-refresh the charts

    targets: [],                    // an array of all the state objects that are
                                    // currently active (independently of their
                                    // viewport visibility)

    updated_dom: true,              // when true, the DOM has been updated with
                                    // new elements we have to check.

    auto_refresher_fast_weight: 0,  // this is the current time in ms, spent
                                    // rendering charts continuously.
                                    // used with .current.fast_render_timeframe

    page_is_visible: true,          // when true, this page is visible

    auto_refresher_stop_until: 0,   // timestamp in ms - used internally, to stop the
                                    // auto-refresher for some time (when a chart is
                                    // performing pan or zoom, we need to stop refreshing
                                    // all other charts, to have the maximum speed for
                                    // rendering the chart that is panned or zoomed).
                                    // Used with .current.global_pan_sync_time

    on_scroll_refresher_stop_until: 0, // timestamp in ms - used to stop evaluating
    // charts for some time, after a page scroll

    last_page_resize: Date.now(),   // the timestamp of the last resize request

    last_page_scroll: 0,            // the timestamp the last time the page was scrolled

    browser_timezone: 'unknown',    // timezone detected by javascript
    server_timezone: 'unknown',     // timezone reported by the server

    force_data_points: 0,           // force the number of points to be returned for charts
    fake_chart_rendering: false,    // when set to true, the dashboard will download data but will not render the charts

    passive_events: null,           // true if the browser supports passive events

    // the current profile
    // we may have many...
    current: {
        units: 'auto',              // can be 'auto' or 'original'
        temperature: 'celsius',     // can be 'celsius' or 'fahrenheit'
        seconds_as_time: true,      // show seconds as DDd:HH:MM:SS ?
        timezone: 'default',        // the timezone to use, or 'default'
        user_set_server_timezone: 'default', // as set by the user on the dashboard

        legend_toolbox: true,       // show the legend toolbox on charts
        resize_charts: true,        // show the resize handler on charts

        pixels_per_point: isSlowDevice() ? 5 : 1, // the minimum pixels per point for all charts
        // increase this to speed javascript up
        // each chart library has its own limit too
        // the max of this and the chart library is used
        // the final is calculated every time, so a change
        // here will have immediate effect on the next chart
        // update

        idle_between_charts: 100,   // ms - how much time to wait between chart updates

        fast_render_timeframe: 200, // ms - render continuously until this time of continuous
                                    // rendering has been reached
                                    // this setting is used to make it render e.g. 10
                                    // charts at once, sleep idle_between_charts time
                                    // and continue for another 10 charts.

        idle_between_loops: 500,    // ms - if all charts have been updated, wait this
                                    // time before starting again.

        idle_parallel_loops: 100,   // ms - the time between parallel refresher updates

        idle_lost_focus: 500,       // ms - when the window does not have focus, check
                                    // if focus has been regained, every this time

        global_pan_sync_time: 300,  // ms - when you pan or zoom a chart, the background
                                    // auto-refreshing of charts is paused for this amount
                                    // of time

        sync_selection_delay: 400,  // ms - when you pan or zoom a chart, wait this amount
                                    // of time before setting up synchronized selections
                                    // on hover.

        sync_selection: true,       // enable or disable selection sync

        pan_and_zoom_delay: 50,     // when panning or zooming, how ofter to update the chart

        sync_pan_and_zoom: true,    // enable or disable pan and zoom sync

        pan_and_zoom_data_padding: true, // fetch more data for the master chart when panning or zooming

        update_only_visible: true,  // enable or disable visibility management / used for printing

        parallel_refresher: !isSlowDevice(), // enable parallel refresh of charts

        concurrent_refreshes: true, // when parallel_refresher is enabled, sync also the charts

        destroy_on_hide: isSlowDevice(), // destroy charts when they are not visible

        show_help: netdataShowHelp, // when enabled the charts will show some help
        show_help_delay_show_ms: 500,
        show_help_delay_hide_ms: 0,

        eliminate_zero_dimensions: true, // do not show dimensions with just zeros

        stop_updates_when_focus_is_lost: true, // boolean - shall we stop auto-refreshes when document does not have user focus
        stop_updates_while_resizing: 1000,  // ms - time to stop auto-refreshes while resizing the charts

        double_click_speed: 500,    // ms - time between clicks / taps to detect double click/tap

        smooth_plot: !isSlowDevice(), // enable smooth plot, where possible

        color_fill_opacity_line: 1.0,
        color_fill_opacity_area: 0.2,
        color_fill_opacity_stacked: 0.8,

        pan_and_zoom_factor: 0.25,      // the increment when panning and zooming with the toolbox
        pan_and_zoom_factor_multiplier_control: 2.0,
        pan_and_zoom_factor_multiplier_shift: 3.0,
        pan_and_zoom_factor_multiplier_alt: 4.0,

        abort_ajax_on_scroll: false,            // kill pending ajax page scroll
        async_on_scroll: false,                 // sync/async onscroll handler
        onscroll_worker_duration_threshold: 30, // time in ms, for async scroll handler

        retries_on_data_failures: 3, // how many retries to make if we can't fetch chart data from the server

        setOptionCallback: function () {
        }
    },

    debug: {
        show_boxes: false,
        main_loop: false,
        focus: false,
        visibility: false,
        chart_data_url: false,
        chart_errors: true, // remember to set it to false before merging
        chart_timing: false,
        chart_calls: false,
        libraries: false,
        dygraph: false,
        globalSelectionSync: false,
        globalPanAndZoom: false
    }
};

NETDATA.statistics = {
    refreshes_total: 0,
    refreshes_active: 0,
    refreshes_active_max: 0
};
