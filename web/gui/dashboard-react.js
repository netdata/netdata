/**
 *  after react-dashboard refractor, this file can be renamed to 'dashboard.js'
 *  and it will:
 *  - setup global objects, so any assignements like 'NETDATA.options.current.destroy_on_hide = true'
 *    will not break. we need to add it in places where 'dashboard.js' is
 *  - create react root DOM node
 *  - load react app
 *
 *  Later, for performance improvement, the bundle can be added to dashboard-rect.js,
 *  but we need to run the react-app part after DOM is created and ready
 */

// ----------------------------------------------------------------------------
// global namespace
/* eslint-disable no-undef */

// Should stay var!
var NETDATA = window.NETDATA || {};

/// A heuristic for detecting slow devices.
let isSlowDeviceResult;
const isSlowDevice = function () {
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

if (typeof window.netdataSnapshotData === 'undefined') {
  window.netdataSnapshotData = null;
}

if (typeof window.netdataShowHelp === 'undefined') {
  window.netdataShowHelp = true;
}

if (typeof window.netdataShowAlarms === 'undefined') {
  window.netdataShowAlarms = false;
}

if (typeof window.netdataRegistryAfterMs !== 'number' || window.netdataRegistryAfterMs < 0) {
  window.netdataRegistryAfterMs = 0; // 1500;
}

if (typeof window.netdataRegistry === 'undefined') {
  // backward compatibility
  window.netdataRegistry = (typeof netdataNoRegistry !== 'undefined' && netdataNoRegistry === false);
}

if (window.netdataRegistry === false && typeof netdataRegistryCallback === 'function') {
  window.netdataRegistry = true;
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

  browser_timezone: (Intl && Intl.DateTimeFormat)
    ? Intl.DateTimeFormat().resolvedOptions().timeZone // timezone detected by javascript
    : "cannot-detect-it",

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

    // when enabled the charts will show some help
    // when there's no bootstrap, we can't show it
    show_help: netdataShowHelp && !window.netdataNoBootstrap,
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

// todo
const serverStatic = '/'
NETDATA.themes = {
  white: {
    bootstrap_css: serverStatic + 'css/bootstrap-3.3.7.css',
    dashboard_css: serverStatic + 'dashboard.css?v20180210-1',
    background: '#FFFFFF',
    foreground: '#000000',
    grid: '#F0F0F0',
    axis: '#F0F0F0',
    highlight: '#F5F5F5',
    colors: ['#3366CC', '#DC3912', '#109618', '#FF9900', '#990099', '#DD4477',
      '#3B3EAC', '#66AA00', '#0099C6', '#B82E2E', '#AAAA11', '#5574A6',
      '#994499', '#22AA99', '#6633CC', '#E67300', '#316395', '#8B0707',
      '#329262', '#3B3EAC'],
    easypiechart_track: '#f0f0f0',
    easypiechart_scale: '#dfe0e0',
    gauge_pointer: '#C0C0C0',
    gauge_stroke: '#F0F0F0',
    gauge_gradient: false,
    d3pie: {
      title: '#333333',
      subtitle: '#666666',
      footer: '#888888',
      other: '#aaaaaa',
      mainlabel: '#333333',
      percentage: '#dddddd',
      value: '#aaaa22',
      tooltip_bg: '#000000',
      tooltip_fg: '#efefef',
      segment_stroke: "#ffffff",
      gradient_color: '#000000'
    }
  },
  slate: {
    bootstrap_css: serverStatic + 'css/bootstrap-slate-flat-3.3.7.css?v20161229-1',
    dashboard_css: serverStatic + 'dashboard.slate.css?v20180210-1',
    background: '#272b30',
    foreground: '#C8C8C8',
    grid: '#283236',
    axis: '#283236',
    highlight: '#383838',
    /*          colors: [   '#55bb33', '#ff2222',   '#0099C6', '#faa11b',   '#adbce0', '#DDDD00',
                        '#4178ba', '#f58122',   '#a5cc39', '#f58667',   '#f5ef89', '#cf93c0',
                        '#a5d18a', '#b8539d',   '#3954a3', '#c8a9cf',   '#c7de8a', '#fad20a',
                        '#a6a479', '#a66da8' ],
    */
    colors: ['#66AA00', '#FE3912', '#3366CC', '#D66300', '#0099C6', '#DDDD00',
      '#5054e6', '#EE9911', '#BB44CC', '#e45757', '#ef0aef', '#CC7700',
      '#22AA99', '#109618', '#905bfd', '#f54882', '#4381bf', '#ff3737',
      '#329262', '#3B3EFF'],
    easypiechart_track: '#373b40',
    easypiechart_scale: '#373b40',
    gauge_pointer: '#474b50',
    gauge_stroke: '#373b40',
    gauge_gradient: false,
    d3pie: {
      title: '#C8C8C8',
      subtitle: '#283236',
      footer: '#283236',
      other: '#283236',
      mainlabel: '#C8C8C8',
      percentage: '#dddddd',
      value: '#cccc44',
      tooltip_bg: '#272b30',
      tooltip_fg: '#C8C8C8',
      segment_stroke: "#283236",
      gradient_color: '#000000'
    }
  }
};

// Codacy declarations
/* global netdataTheme */
if (typeof window.netdataTheme !== 'undefined'
  && typeof NETDATA.themes[netdataTheme] !== 'undefined'
) {
  NETDATA.themes.current = NETDATA.themes[window.netdataTheme];
} else {
  NETDATA.themes.current = NETDATA.themes.white;
}

NETDATA.colors = NETDATA.themes.current.colors;

// these are the colors Google Charts are using
// we have them here to attempt emulate their look and feel on the other chart libraries
// http://there4.io/2012/05/02/google-chart-color-list/
//NETDATA.colors        = [ '#3366CC', '#DC3912', '#FF9900', '#109618', '#990099', '#3B3EAC', '#0099C6',
//                      '#DD4477', '#66AA00', '#B82E2E', '#316395', '#994499', '#22AA99', '#AAAA11',
//                      '#6633CC', '#E67300', '#8B0707', '#329262', '#5574A6', '#3B3EAC' ];

// an alternative set
// http://www.mulinblog.com/a-color-palette-optimized-for-data-visualization/
//                         (blue)     (red)      (orange)   (green)    (pink)     (brown)    (purple)   (yellow)   (gray)
//NETDATA.colors        = [ '#5DA5DA', '#F15854', '#FAA43A', '#60BD68', '#F17CB0', '#B2912F', '#B276B2', '#DECF3F', '#4D4D4D' ];
// dygraph

// local storage options

NETDATA.localStorage = {
  default: {},
  current: {},
  callback: {} // only used for resetting back to defaults
};
