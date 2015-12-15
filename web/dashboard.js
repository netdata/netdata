// You can set the following variables before loading this script:
//
// var netdataNoDygraphs = true;		// do not use dygraph
// var netdataNoSparklines = true;		// do not use sparkline
// var netdataNoPeitys = true;			// do not use peity
// var netdataNoGoogleCharts = true;	// do not use google
// var netdataNoMorris = true;			// do not use morris
// var netdataDontStart = true;			// do not start the thread to process the charts
//
// You can also set the default netdata server, using the following.
// When this variable is not set, we assume the page is hosted on your
// netdata server already.
// var netdataServer = "http://yourhost:19999"; // set your NetData server

//(function(window, document, undefined) {
	// fix IE bug with console
	if(!window.console){ window.console = {log: function(){} }; }

	// global namespace
	var NETDATA = window.NETDATA || {};

	// ----------------------------------------------------------------------------------------------------------------
	// Detect the netdata server

	// http://stackoverflow.com/questions/984510/what-is-my-script-src-url
	// http://stackoverflow.com/questions/6941533/get-protocol-domain-and-port-from-url
	NETDATA._scriptSource = function(scripts) {
		var script = null, base = null;

		if(typeof document.currentScript !== 'undefined') {
			script = document.currentScript;
		}
		else {
			var all_scripts = document.getElementsByTagName('script');
			script = all_scripts[all_scripts.length - 1];
		}

		if (typeof script.getAttribute.length !== 'undefined')
			script = script.src;
		else
			script = script.getAttribute('src', -1);

		var link = document.createElement('a');
		link.setAttribute('href', script);

		if(!link.protocol || !link.hostname) return null;

		base = link.protocol;
		if(base) base += "//";
		base += link.hostname;

		if(link.port) base += ":" + link.port;
		base += "/";

		return base;
	};

	if(typeof netdataServer !== 'undefined')
		NETDATA.serverDefault = netdataServer;
	else
		NETDATA.serverDefault = NETDATA._scriptSource();

	if(NETDATA.serverDefault === null)
		NETDATA.serverDefault = '';
	else if(NETDATA.serverDefault.slice(-1) !== '/')
		NETDATA.serverDefault += '/';

	// default URLs for all the external files we need
	// make them RELATIVE so that the whole thing can also be
	// installed under a web server
	NETDATA.jQuery       		= NETDATA.serverDefault + 'lib/jquery-1.11.3.min.js';
	NETDATA.peity_js     		= NETDATA.serverDefault + 'lib/jquery.peity.min.js';
	NETDATA.sparkline_js 		= NETDATA.serverDefault + 'lib/jquery.sparkline.min.js';
	NETDATA.easypiechart_js 	= NETDATA.serverDefault + 'lib/jquery.easypiechart.min.js';
	NETDATA.dygraph_js   		= NETDATA.serverDefault + 'lib/dygraph-combined.js';
	NETDATA.dygraph_smooth_js   = NETDATA.serverDefault + 'lib/dygraph-smooth-plotter.js';
	NETDATA.raphael_js   		= NETDATA.serverDefault + 'lib/raphael-min.js';
	NETDATA.morris_js    		= NETDATA.serverDefault + 'lib/morris.min.js';
	NETDATA.morris_css   		= NETDATA.serverDefault + 'css/morris.css';
	NETDATA.dashboard_css		= NETDATA.serverDefault + 'dashboard.css';
	NETDATA.google_js    		= 'https://www.google.com/jsapi';

	// these are the colors Google Charts are using
	// we have them here to attempt emulate their look and feel on the other chart libraries
	// http://there4.io/2012/05/02/google-chart-color-list/
	//NETDATA.colors		= [ '#3366CC', '#DC3912', '#FF9900', '#109618', '#990099', '#3B3EAC', '#0099C6',
	//						'#DD4477', '#66AA00', '#B82E2E', '#316395', '#994499', '#22AA99', '#AAAA11',
	//						'#6633CC', '#E67300', '#8B0707', '#329262', '#5574A6', '#3B3EAC' ];

	NETDATA.colors		= [ '#3366CC', '#DC3912', '#109618', '#FF9900', '#990099', '#DD4477', '#3B3EAC',
							'#66AA00', '#0099C6', '#B82E2E', '#AAAA11', '#5574A6', '#994499', '#22AA99',
							'#6633CC', '#E67300', '#316395', '#8B0707', '#329262', '#3B3EAC' ];
	// an alternative set
	// http://www.mulinblog.com/a-color-palette-optimized-for-data-visualization/
	//                         (blue)     (red)      (orange)   (green)    (pink)     (brown)    (purple)   (yellow)   (gray)
	//NETDATA.colors 		= [ '#5DA5DA', '#F15854', '#FAA43A', '#60BD68', '#F17CB0', '#B2912F', '#B276B2', '#DECF3F', '#4D4D4D' ];

	// ----------------------------------------------------------------------------------------------------------------
	// the defaults for all charts

	// if the user does not specify any of these, the following will be used

	NETDATA.chartDefaults = {
		host: NETDATA.serverDefault,	// the server to get data from
		width: '100%',					// the chart width - can be null
		height: '100%',					// the chart height - can be null
		min_width: null,				// the chart minimum width - can be null
		library: 'dygraph',				// the graphing library to use
		method: 'average',				// the grouping method
		before: 0,						// panning
		after: -600,					// panning
		pixels_per_point: 1,			// the detail of the chart
		fill_luminance: 0.8				// luminance of colors in solit areas
	}

	// ----------------------------------------------------------------------------------------------------------------
	// global options

	NETDATA.options = {
		readyCallback: null,			// a callback when we load the required stuf
		pauseCallback: null,			// a callback when we are really paused

		pause: false,					// when enabled we don't auto-refresh the charts

		targets: null,					// an array of all the state objects that are
										// currently active (independently of their
										// viewport visibility)

		updated_dom: true,				// when true, the DOM has been updated with
										// new elements we have to check.

		auto_refresher_fast_weight: 0,	// this is the current time in ms, spent
										// rendering charts continiously.
										// used with .current.fast_render_timeframe

		page_is_visible: true,			// when true, this page is visible

		auto_refresher_stop_until: 0,	// timestamp in ms - used internaly, to stop the
										// auto-refresher for some time (when a chart is
										// performing pan or zoom, we need to stop refreshing
										// all other charts, to have the maximum speed for
										// rendering the chart that is panned or zoomed).
										// Used with .current.global_pan_sync_time

		last_resized: 0,				// the timestamp of the last resize request

		crossDomainAjax: false,			// enable this to request crossDomain AJAX

		last_page_scroll: 0,			// the timestamp the last time the page was scrolled

		// the current profile
		// we may have many...
		current: {
			pixels_per_point: 1,		// the minimum pixels per point for all charts
										// increase this to speed javascript up
										// each chart library has its own limit too
										// the max of this and the chart library is used
										// the final is calculated every time, so a change
										// here will have immediate effect on the next chart
										// update

			idle_between_charts: 100,	// ms - how much time to wait between chart updates

			fast_render_timeframe: 100, // ms - render continously until this time of continious
										// rendering has been reached
										// this setting is used to make it render e.g. 10
										// charts at once, sleep idle_between_charts time
										// and continue for another 10 charts.

			idle_between_loops: 200,	// ms - if all charts have been updated, wait this
										// time before starting again.

			idle_lost_focus: 500,		// ms - when the window does not have focus, check
										// if focus has been regained, every this time

			global_pan_sync_time: 1500,	// ms - when you pan or zoon a chart, the background
										// autorefreshing of charts is paused for this amount
										// of time

			sync_selection_delay: 2500,	// ms - when you pan or zoom a chart, wait this amount
										// of time before setting up synchronized selections
										// on hover.

			sync_selection: true,		// enable or disable selection sync

			pan_and_zoom_delay: 50,		// when panning or zooming, how ofter to update the chart

			sync_pan_and_zoom: true,	// enable or disable pan and zoom sync

			update_only_visible: true,	// enable or disable visibility management

			parallel_refresher: true,	// enable parallel refresh of charts

			destroy_on_hide: false,		// destroy charts when they are not visible

			color_fill_opacity: {
				line: 1.0,
				area: 0.2,
				stacked: 0.8
			}
		},

		debug: {
			show_boxes: 		0,
			main_loop: 			0,
			focus: 				0,
			visibility: 		0,
			chart_data_url: 	0,
			chart_errors: 		1,
			chart_timing: 		0,
			chart_calls: 		0,
			libraries: 			0,
			dygraph: 			0
		}
	}

	if(NETDATA.options.debug.main_loop) console.log('welcome to NETDATA');

	window.onresize = function(event) {
		NETDATA.options.last_page_scroll = new Date().getTime();
		NETDATA.options.last_resized = new Date().getTime();
	};

	window.onscroll = function(event) {
		NETDATA.options.last_page_scroll = new Date().getTime();

		// when the user scrolls he sees that we have
		// hidden all the not-visible charts
		// using this little function we try to switch
		// the charts back to visible quickly
		var targets = NETDATA.options.targets;
		var len = targets.length;
		while(len--) targets[len].isVisible();
	}

	// ----------------------------------------------------------------------------------------------------------------
	// Error Handling

	NETDATA.errorCodes = {
		100: { message: "Cannot load chart library", alert: true },
		101: { message: "Cannot load jQuery", alert: true },
		402: { message: "Chart library not found", alert: false },
		403: { message: "Chart library not enabled/is failed", alert: false },
		404: { message: "Chart not found", alert: false }
	};
	NETDATA.errorLast = {
		code: 0,
		message: "",
		datetime: 0
	};

	NETDATA.error = function(code, msg) {
		NETDATA.errorLast.code = code;
		NETDATA.errorLast.message = msg;
		NETDATA.errorLast.datetime = new Date().getTime();

		console.log("ERROR " + code + ": " + NETDATA.errorCodes[code].message + ": " + msg);

		if(NETDATA.errorCodes[code].alert)
			alert("ERROR " + code + ": " + NETDATA.errorCodes[code].message + ": " + msg);
	}

	NETDATA.errorReset = function() {
		NETDATA.errorLast.code = 0;
		NETDATA.errorLast.message = "You are doing fine!";
		NETDATA.errorLast.datetime = 0;
	};

	// ----------------------------------------------------------------------------------------------------------------
	// Chart Registry

	// When multiple charts need the same chart, we avoid downloading it
	// multiple times (and having it in browser memory multiple time)
	// by using this registry.

	// Every time we download a chart definition, we save it here with .add()
	// Then we try to get it back with .get(). If that fails, we download it.

	NETDATA.chartRegistry = {
		charts: {},

		fixid: function(id) {
			return id.replace(/:/g, "_").replace(/\//g, "_");
		},

		add: function(host, id, data) {
			host = this.fixid(host);
			id   = this.fixid(id);

			if(typeof this.charts[host] === 'undefined')
				this.charts[host] = {};

			//console.log('added ' + host + '/' + id);
			this.charts[host][id] = data;
		},

		get: function(host, id) {
			host = this.fixid(host);
			id   = this.fixid(id);

			if(typeof this.charts[host] === 'undefined')
				return null;

			if(typeof this.charts[host][id] === 'undefined')
				return null;

			//console.log('cached ' + host + '/' + id);
			return this.charts[host][id];
		},

		downloadAll: function(host, callback) {
			while(host.slice(-1) === '/')
				host = host.substring(0, host.length - 1);

			self = this;

			$.ajax({
				url: host + '/api/v1/charts',
				crossDomain: NETDATA.options.crossDomainAjax,
				async: true,
				cache: false
			})
			.done(function(data) {
				var h = NETDATA.chartRegistry.fixid(host);
				//console.log('downloaded all charts from ' + host + ' (' + h + ')');
				self.charts[h] = data.charts;
				if(typeof callback === 'function')
					callback(data);
			})
			.fail(function() {
				if(typeof callback === 'function')
					callback(null);
			});
		}
	};

	// ----------------------------------------------------------------------------------------------------------------
	// Global Pan and Zoom on charts

	// Using this structure are synchronize all the charts, so that
	// when you pan or zoom one, all others are automatically refreshed
	// to the same timespan.

	NETDATA.globalPanAndZoom = {
		seq: 0,					// timestamp ms
								// every time a chart is panned or zoomed
								// we set the timestamp here
								// then we use it as a sequence number
								// to find if other charts are syncronized
								// to this timerange

		master: null,			// the master chart (state), to which all others
								// are synchronized

		force_before_ms: null,	// the timespan to sync all other charts 
		force_after_ms: null,

		// set a new master
		setMaster: function(state, after, before) {
			if(!NETDATA.options.current.sync_pan_and_zoom) return;

			if(this.master !== null && this.master !== state)
				this.master.resetChart();

			var now = new Date().getTime();
			this.master = state;
			this.seq = now;
			this.force_after_ms = after;
			this.force_before_ms = before;
			NETDATA.options.auto_refresher_stop_until = now + NETDATA.options.current.global_pan_sync_time;
		},

		// clear the master
		clearMaster: function() {
			if(!NETDATA.options.current.sync_pan_and_zoom) return;

			if(this.master !== null) {
				var state = this.master;
				this.master = null; // prevent infinite recursion
				this.seq = 0;
				state.resetChart();
				NETDATA.options.auto_refresher_stop_until = 0;
			}

			this.master = null;
			this.seq = 0;
			this.force_after_ms = null;
			this.force_before_ms = null;
		},

		// is the given state the master of the global
		// pan and zoom sync?
		isMaster: function(state) {
			if(this.master === state) return true;
			return false;
		},

		// are we currently have a global pan and zoom sync?
		isActive: function() {
			if(this.master !== null && this.force_before_ms !== null && this.force_after_ms !== null && this.seq !== 0) return true;
			return false;
		},

		// check if a chart, other than the master
		// needs to be refreshed, due to the global pan and zoom
		shouldBeAutoRefreshed: function(state) {
			if(this.master === null || this.seq === 0)
				return false;

			if(state.needsResize())
				return true;

			if(state.tm.pan_and_zoom_seq === this.seq)
				return false;

			return true;
		}
	}

	// ----------------------------------------------------------------------------------------------------------------
	// Our state object, where all per-chart values are stored

	chartState = function(element) {
		self = $(element);

		$.extend(this, {
			uuid: NETDATA.guid(),	// GUID - a unique identifier for the chart
			id: self.data('netdata'),	// string - the name of chart

			// the user given dimensions of the element
			width: self.data('width') || NETDATA.chartDefaults.width,
			height: self.data('height') || NETDATA.chartDefaults.height,

			// string - the netdata server URL, without any path
			host: self.data('host') || NETDATA.chartDefaults.host,

			// string - the grouping method requested by the user
			method: self.data('method') || NETDATA.chartDefaults.method,

			// the time-range requested by the user
			after: self.data('after') || NETDATA.chartDefaults.after,
			before: self.data('before') || NETDATA.chartDefaults.before,

			// the pixels per point requested by the user
			pixels_per_point: self.data('pixels-per-point') || 1,
			points: self.data('points') || null,

			// the dimensions requested by the user
			dimensions: self.data('dimensions') || null,

			// the chart library requested by the user
			library_name: self.data('chart-library') || NETDATA.chartDefaults.library,
			library: null,			// object - the chart library used

			colors: null,

			element: element,		// the element already created by the user
			element_message: null,
			element_loading: null,
			element_chart: null,	// the element with the chart
			element_chart_id: null,
			element_legend: null, 	// the element with the legend of the chart (if created by us)
			element_legend_id: null,
			element_legend_childs: {
				hidden: null,
				title_date: null,
				title_time: null,
				title_units: null,
				nano: null,
				nano_options: null,
				series: null
			},

			chart_url: null,		// string - the url to download chart info
			chart: null,			// object - the chart as downloaded from the server

			validated: false, 		// boolean - has the chart been validated?
			enabled: true, 			// boolean - is the chart enabled for refresh?
			paused: false,			// boolean - is the chart paused for any reason?
			selected: false,		// boolean - is the chart shown a selection?
			debug: false,			// boolean - console.log() debug info about this chart

			updates_counter: 0,		// numeric - the number of refreshes made so far
			updates_since_last_creation: 0,

			tm: {
				last_info_downloaded: 0,	// milliseconds - the timestamp we downloaded the chart

				last_created: 0,			// the timestamp the chart was created

				last_updated: 0,			// the timestamp the chart last updated with data

				pan_and_zoom_seq: 0,		// the sequence number of the global synchronization
											// between chart.
											// Used with NETDATA.globalPanAndZoom.seq

				last_visible_check: 0,		// the time we last checked if it is visible

				last_resized: 0,			// the time the chart was resized
				last_hidden: 0,				// the time the chart was hidden
				last_unhidden: 0,			// the time the chart was unhidden
			},

			current: null, 			// auto, pan, zoom
									// this is a pointer to one of the sub-classes below

			auto: {
				name: 'auto',
				autorefresh: true,
				url: 'invalid://',	// string - the last url used to update the chart
				last_autorefreshed: 0, // milliseconds - the timestamp of last automatic refresh
				view_update_every: 0, 	// milliseconds - the minimum acceptable refresh duration
				after_ms: 0,		// milliseconds - the first timestamp of the data
				before_ms: 0,		// milliseconds - the last timestamp of the data
				points: 0,			// number - the number of points in the data
				data: null,			// the last downloaded data
				force_update_at: 0, // the timestamp to force the update at
				force_before_ms: null,
				force_after_ms: null,
				requested_before_ms: null,
				requested_after_ms: null,
				first_entry_ms: null,
				last_entry_ms: null
			},
			pan: {
				name: 'pan',
				autorefresh: false,
				url: 'invalid://',	// string - the last url used to update the chart
				last_autorefreshed: 0, // milliseconds - the timestamp of last refresh
				view_update_every: 0, 	// milliseconds - the minimum acceptable refresh duration
				after_ms: 0,		// milliseconds - the first timestamp of the data
				before_ms: 0,		// milliseconds - the last timestamp of the data
				points: 0,			// number - the number of points in the data
				data: null,			// the last downloaded data
				force_update_at: 0, // the timestamp to force the update at
				force_before_ms: null,
				force_after_ms: null,
				requested_before_ms: null,
				requested_after_ms: null,
				first_entry_ms: null,
				last_entry_ms: null
			},
			zoom: {
				name: 'zoom',
				autorefresh: false,
				url: 'invalid://',	// string - the last url used to update the chart
				last_autorefreshed: 0, // milliseconds - the timestamp of last refresh
				view_update_every: 0, 	// milliseconds - the minimum acceptable refresh duration
				after_ms: 0,		// milliseconds - the first timestamp of the data
				before_ms: 0,		// milliseconds - the last timestamp of the data
				points: 0,			// number - the number of points in the data
				data: null,			// the last downloaded data
				force_update_at: 0, // the timestamp to force the update at
				force_before_ms: null,
				force_after_ms: null,
				requested_before_ms: null,
				requested_after_ms: null,
				first_entry_ms: null,
				last_entry_ms: null
			},

			refresh_dt_ms: 0,		// milliseconds - the time the last refresh took
			refresh_dt_element_name: self.data('dt-element-name') || null,	// string - the element to print refresh_dt_ms
			refresh_dt_element: null
		});

		this.init();
	}

	// ----------------------------------------------------------------------------------------------------------------
	// global selection sync

	NETDATA.globalSelectionSync = {
		state: null,
		dont_sync_before: 0,
		slaves: []
	};

	// prevent to global selection sync for some time
	chartState.prototype.globalSelectionSyncDelay = function(ms) {
		if(!NETDATA.options.current.sync_selection) return;
		if(typeof ms === 'number')
			NETDATA.globalSelectionSync.dont_sync_before = new Date().getTime() + ms;
		else
			NETDATA.globalSelectionSync.dont_sync_before = new Date().getTime() + NETDATA.options.current.sync_selection_delay;
	}

	// can we globally apply selection sync?
	chartState.prototype.globalSelectionSyncAbility = function() {
		if(!NETDATA.options.current.sync_selection) return false;
		if(NETDATA.globalSelectionSync.dont_sync_before > new Date().getTime()) return false;
		return true;
	}

	chartState.prototype.globalSelectionSyncIsMaster = function() {
		if(NETDATA.globalSelectionSync.state === this)
			return true;
		else
			return false;
	}

	// this chart is the master of the global selection sync
	chartState.prototype.globalSelectionSyncBeMaster = function() {
		// am I the master?
		if(this.globalSelectionSyncIsMaster()) {
			if(this.debug) this.log('sync: I am the master already.');
			return;
		}

		if(NETDATA.globalSelectionSync.state) {
			if(this.debug) this.log('sync: I am not the sync master. Resetting global sync.');
			this.globalSelectionSyncStop();
		}

		// become the master
		if(this.debug) this.log('sync: becoming sync master.');
		this.selected = true;
		NETDATA.globalSelectionSync.state = this;

		// find the all slaves
		var targets = NETDATA.options.targets;
		var len = targets.length;
		while(len--) {
			st = targets[len];

			if(st === this) {
				if(this.debug) st.log('sync: not adding me to sync');
			}
			else if(st.globalSelectionSyncIsEligible()) {
				if(this.debug) st.log('sync: adding to sync as slave');
				st.globalSelectionSyncBeSlave();
			}
		}

		// this.globalSelectionSyncDelay(100);
	}

	// can the chart participate to the global selection sync as a slave?
	chartState.prototype.globalSelectionSyncIsEligible = function() {
		if(this.library !== null && typeof this.library.setSelection === 'function' && this.isVisible() && this.tm.last_created !== 0)
			return true;
		return false;
	}

	// this chart is a slave of the global selection sync
	chartState.prototype.globalSelectionSyncBeSlave = function() {
		if(NETDATA.globalSelectionSync.state !== this)
			NETDATA.globalSelectionSync.slaves.push(this);
	}

	// sync all the visible charts to the given time
	// this is to be called from the chart libraries
	chartState.prototype.globalSelectionSync = function(t) {
		if(!this.globalSelectionSyncAbility()) {
			if(this.debug) this.log('sync: cannot sync (yet?).');
			return;
		}

		if(!this.globalSelectionSyncIsMaster()) {
			if(this.debug) this.log('sync: trying to be sync master.');
			this.globalSelectionSyncBeMaster();

			if(!this.globalSelectionSyncAbility()) {
				if(this.debug) this.log('sync: cannot sync (yet?).');
				return;
			}
		}

		// FIXME
		// var start = new Date().getTime();

		$.each(NETDATA.globalSelectionSync.slaves, function(i, st) {
			st.setSelection(t);
		});

		// FIXME
		// var end = new Date().getTime();
		// console.log(end - start);
	}

	// stop syncing all charts to the given time
	chartState.prototype.globalSelectionSyncStop = function() {
		if(NETDATA.globalSelectionSync.slaves.length) {
			if(this.debug) this.log('sync: cleaning up...');
			var self = this;
			$.each(NETDATA.globalSelectionSync.slaves, function(i, st) {
				if(st === self) {
					if(self.debug) st.log('sync: not adding me to sync stop');
				}
				else {
					if(self.debug) st.log('sync: removed slave from sync');
					st.clearSelection();
				}
			});

			NETDATA.globalSelectionSync.slaves = [];
			NETDATA.globalSelectionSync.state = null;
		}

		// since we are the sync master, we should not call this.clearSelection()
		// dygraphs is taking care of visualizing our selection.
		this.selected = false;
	}

	chartState.prototype.setSelection = function(t) {
		if(typeof this.library.setSelection === 'function') {
			if(this.library.setSelection(this, t))
				this.selected = true;
			else
				this.selected = false;
		}
		else this.selected = true;

		if(this.selected && this.debug) this.log('selection set to ' + t.toString());

		return this.selected;
	}

	chartState.prototype.clearSelection = function() {
		if(this.selected) {
			if(typeof this.library.clearSelection === 'function') {
				if(this.library.clearSelection(this))
					this.selected = false;
				else
					this.selected = true;
			}
			else this.selected = false;
			
			if(!this.selected && this.debug) this.log('selection cleared');
		}

		this.legendReset();
		return this.selected;
	}

	// find if a timestamp (ms) is shown in the current chart
	chartState.prototype.timeIsVisible = function(t) {
		if(t >= this.current.after_ms && t <= this.current.before_ms)
			return true;
		return false;
	},

	chartState.prototype.calculateRowForTime = function(t) {
		if(!this.timeIsVisible(t)) return -1;
		return Math.floor((t - this.current.after_ms) / this.current.view_update_every);
	}

	// ----------------------------------------------------------------------------------------------------------------

	// console logging
	chartState.prototype.log = function(msg) {
		console.log(this.id + ' (' + this.library_name + ' ' + this.uuid + '): ' + msg);
	}

	chartState.prototype.pauseChart = function() {
		if(!this.paused) {
			if(this.debug) this.log('paused');
			this.paused = true;
		}
	}

	chartState.prototype.unpauseChart = function() {
		if(this.paused) {
			if(this.debug) this.log('unpaused');
			this.paused = false;
		}
	}

	chartState.prototype.resetChart = function() {
		if(NETDATA.globalPanAndZoom.isMaster(this) && this.isVisible())
			NETDATA.globalPanAndZoom.clearMaster();

		this.tm.pan_and_zoom_seq = 0;

		this.clearSelection();

		this.setMode('auto');
		this.current.force_update_at = 0;
		this.current.force_before_ms = null;
		this.current.force_after_ms = null;
		this.current.last_autorefreshed = 0;
		this.paused = false;
		this.selected = false;
		this.enabled = true;
		this.debug = false;

		// do not update the chart here
		// or the chart will flip-flop when it is the master
		// of a selection sync and another chart becomes
		// the new master
		if(!NETDATA.options.current.sync_pan_and_zoom && this.isVisible())
			state.updateChart();
	}

	chartState.prototype.setMode = function(m) {
		if(this.current) {
			if(this.current.name === m) return;

			this[m].url = this.current.url;
			this[m].last_autorefreshed = this.current.last_autorefreshed;
			this[m].view_update_every = this.current.view_update_every;
			this[m].after_ms = this.current.after_ms;
			this[m].before_ms = this.current.before_ms;
			this[m].points = this.current.points;
			this[m].data = this.current.data;
			this[m].requested_before_ms = this.current.requested_before_ms;
			this[m].requested_after_ms = this.current.requested_after_ms;
			this[m].first_entry_ms = this.current.first_entry_ms;
			this[m].last_entry_ms = this.current.last_entry_ms;
		}

		if(m === 'auto')
			this.current = this.auto;
		else if(m === 'pan')
			this.current = this.pan;
		else if(m === 'zoom')
			this.current = this.zoom;
		else
			this.current = this.auto;

		this.current.force_update_at = 0;
		this.current.force_before_ms = null;
		this.current.force_after_ms = null;

		if(this.debug) this.log('mode set to ' + this.current.name);
	}

	chartState.prototype._minPanOrZoomStep = function() {
		return (((this.current.before_ms - this.current.after_ms) / this.current.points) * ((this.current.points * 5 / 100) + 1) );
		// return this.current.view_update_every * 10;
	}

	chartState.prototype._shouldBeMoved = function(old_after, old_before, new_after, new_before) {
		var dt_after = Math.abs(old_after - new_after);
		var dt_before = Math.abs(old_before - new_before);
		var old_range = old_before - old_after;

		var new_range = new_before - new_after;
		var dt = Math.abs(old_range - new_range);
		var step = Math.max(dt_after, dt_before, dt);

		var min_step = this._minPanOrZoomStep();
		if(new_range < old_range && new_range / this.chartWidth() < 100) {
			if(this.debug) this.log('_shouldBeMoved(' + (new_after / 1000).toString() + ' - ' + (new_before / 1000).toString() + '): minimum point size: 0.10, wanted point size: ' + (new_range / this.chartWidth() / 1000).toString() + ': TOO SMALL RANGE');
			return false;
		}

		if(step >= min_step) {
			if(this.debug) this.log('_shouldBeMoved(' + (new_after / 1000).toString() + ' - ' + (new_before / 1000).toString() + '): minimum step: ' + (min_step / 1000).toString() + ', this step: ' + (step / 1000).toString() + ': YES');
			return true;
		}
		else {
			if(this.debug) this.log('_shouldBeMoved(' + (new_after / 1000).toString() + ' - ' + (new_before / 1000).toString() + '): minimum step: ' + (min_step / 1000).toString() + ', this step: ' + (step / 1000).toString() + ': NO');
			return false;
		}
	}

	chartState.prototype.updateChartPanOrZoom = function(after, before) {
		var move = false;

		if(this.current.name === 'auto') {
			if(this.debug) this.log('updateChartPanOrZoom(): caller did not set proper mode');
			this.setMode('pan');
		}

		if(!this.current.force_after_ms || !this.current.force_before_ms) {
			if(this.debug) this.log('updateChartPanOrZoom(' + (after / 1000).toString() + ' - ' + (before / 1000).toString() + '): INIT');
			move = true;
		}
		else if(this._shouldBeMoved(this.current.force_after_ms, this.current.force_before_ms, after, before) && this._shouldBeMoved(this.current.after_ms, this.current.before_ms, after, before)) {
			if(this.debug) this.log('updateChartPanOrZoom(' + (after / 1000).toString() + ' - ' + (before / 1000).toString() + '): FORCE CHANGE from ' + (this.current.force_after_ms / 1000).toString() + ' - ' + (this.current.force_before_ms / 1000).toString());
			move = true;
		}
		else if(this._shouldBeMoved(this.current.requested_after_ms, this.current.requested_before_ms, after, before) && this._shouldBeMoved(this.current.after_ms, this.current.before_ms, after, before)) {
			if(this.debug) this.log('updateChartPanOrZoom(' + (after / 1000).toString() + ' - ' + (before / 1000).toString() + '): REQUESTED CHANGE from ' + (this.current.requested_after_ms / 1000).toString() + ' - ' + (this.current.requested_before_ms / 1000).toString());
			move = true;
		}

		if(move) {
			this.current.force_update_at = new Date().getTime() + NETDATA.options.current.pan_and_zoom_delay;
			this.current.force_after_ms = after;
			this.current.force_before_ms = before;
			NETDATA.globalPanAndZoom.setMaster(this, after, before);
			return true;
		}

		if(this.debug) this.log('updateChartPanOrZoom(' + (after / 1000).toString() + ' - ' + (before / 1000).toString() + '): IGNORE');
		return false;
	}

	chartState.prototype.legendFormatValue = function(value) {
		if(value === null) return '-';
		if(typeof value !== 'number') return value;

		var abs = Math.abs(value);
		if(abs >= 1) return (Math.round(value * 100) / 100).toLocaleString();
		if(abs >= 0.1) return (Math.round(value * 1000) / 1000).toLocaleString();
		return (Math.round(value * 10000) / 10000).toLocaleString();
	}

	chartState.prototype.legendSetLabelValue = function(label, value) {
		var series = this.element_legend_childs.series[label];
		if(typeof series === 'undefined') return;
		if(series.value === null && series.user === null) return;

		value = this.legendFormatValue(value);

		// if the value has not changed, skip DOM update
		if(series.last === value) return;
		series.last = value;

		if(series.value !== null) series.value.innerHTML = value;
		if(series.user !== null) series.user.innerHTML = value;
	}

	chartState.prototype.legendSetDate = function(ms) {
		if(typeof ms !== 'number') {
			this.legendUndefined();
			return;
		}

		var d = new Date(ms);

		if(this.element_legend_childs.title_date)
			this.element_legend_childs.title_date.innerHTML = d.toLocaleDateString();

		if(this.element_legend_childs.title_time)
			this.element_legend_childs.title_time.innerHTML = d.toLocaleTimeString();

		if(this.element_legend_childs.title_units)
			this.element_legend_childs.title_units.innerHTML = this.chart.units;
	}

	chartState.prototype.legendUndefined = function() {
		if(this.element_legend_childs.title_date)
			this.element_legend_childs.title_date.innerHTML = '&nbsp;';

		if(this.element_legend_childs.title_time)
			this.element_legend_childs.title_time.innerHTML = this.chart.name;

		if(this.element_legend_childs.title_units)
			this.element_legend_childs.title_units.innerHTML = '&nbsp;';

		if(this.current.data && this.element_legend_childs.series !== null) {
			var labels = this.current.data.dimension_names;
			var i = labels.length;
			while(i--) {
				var label = labels[i];

				if(typeof label === 'undefined') continue;
				if(typeof this.element_legend_childs.series[label] === 'undefined') continue;
				this.legendSetLabelValue(label, null);
			}
		}
	}

	chartState.prototype.legendShowLatestValues = function() {
		if(this.chart === null) return;
		if(this.selected) return;

		if(this.current.data === null || this.element_legend_childs.series === null) {
			this.legendUndefined();
			return;
		}

		var show_undefined = true;
		if(Math.abs(this.current.data.last_entry_t - this.current.data.before) <= this.current.data.view_update_every)
			show_undefined = false;

		if(show_undefined)
			this.legendUndefined();
		else
			this.legendSetDate(this.current.data.before * 1000);

		var labels = this.current.data.dimension_names;
		var i = labels.length;
		while(i--) {
			var label = labels[i];

			if(typeof label === 'undefined') continue;
			if(typeof this.element_legend_childs.series[label] === 'undefined') continue;

			if(show_undefined)
				this.legendSetLabelValue(label, null);
			else
				this.legendSetLabelValue(label, this.current.data.result_latest_values[i]);
		}
	}

	chartState.prototype.legendReset = function() {
		this.legendShowLatestValues();
	}

	chartState.prototype.chartColors = function() {
		if(this.colors !== null) return this.colors;

		this.colors = $(this.element).data('colors');
		if(typeof this.colors === 'undefined' || this.colors === null)
			this.colors = NETDATA.colors;
		else {
			// console.log(this.colors);
			var s = this.colors;
			if(typeof s === 'string') s = s.split(' ');

			this.colors = new Array();
			var self = this;
			$.each(s, function(i, c) {
				self.colors.push(c);
			});

			// push the default colors too
			var self = this;
			$.each(NETDATA.colors, function(i, c) {
				self.colors.push(c);
			});
		}

		return this.colors;
	}

	chartState.prototype.legendUpdateDOM = function() {
		var needed = false;

		// check that the legend DOM is up to date for the downloaded dimensions
		if(typeof this.element_legend_childs.series !== 'object' || this.element_legend_childs.series === null) {
			// this.log('the legend does not have any series - requesting legend update');
			needed = true;
		}
		else if(this.current.data === null) {
			// this.log('the chart does not have any data - requesting legend update');
			needed = true;
		}
		else {
			// this.log('checking existing legend');
			var labels = this.current.data.dimension_names;
			var i = labels.length;
			while(i--) {
				var name = labels[i];
				if(typeof this.element_legend_childs.series[name] === 'undefined') {
					// this.log('legend is incosistent - missing dimension:' + name);
					needed = true;
					break;
				}
				else if(Math.abs(this.current.data.last_entry_t - this.current.data.before) <= this.current.data.view_update_every) {
					// this.log('setting legend of ' + name + ' to ' + this.current.data.latest_values[i]);
					this.legendSetLabelValue(name, this.current.data.latest_values[i]);
				}
			}
		}

		if(!needed) return;

		if(this.debug) this.log('updating Legend DOM');

		self = $(this.element);
		var genLabel = function(state, parent, name, count) {
			var c = count % state.chartColors().length;

			var user_element = null;
			var user_id = self.data('show-value-of-' + name + '-at') || null;
			if(user_id !== null) {
				user_element = document.getElementById(user_id) || null;
				if(user_element === null)
					me.log('Cannot find element with id: ' + user_id);
			}

			state.element_legend_childs.series[name] = {
				name: document.createElement('span'),
				value: document.createElement('span'),
				user: user_element,
				last: null
			};

			var label = state.element_legend_childs.series[name];

			label.name.className += ' netdata-legend-name';
			label.value.className += ' netdata-legend-value';
			label.name.title = name;
			label.value.title = name;

			var rgb = NETDATA.colorHex2Rgb(state.chartColors()[c]);
			label.name.innerHTML = '<table class="netdata-legend-name-table-'
				+ state.chart.chart_type
				+ '" style="background-color: '
				+ 'rgba(' + rgb.r + ',' + rgb.g + ',' + rgb.b + ',' + NETDATA.options.current.color_fill_opacity[state.chart.chart_type] + ')'
				+ '"><tr class="netdata-legend-name-tr"><td class="netdata-legend-name-td"></td></tr></table>'

			var text = document.createTextNode(' ' + name);
			label.name.appendChild(text);

			label.name.style.color = state.chartColors()[c];
			label.value.style.color = state.chartColors()[c];

			if(count > 0)
				parent.appendChild(document.createElement('br'));

			parent.appendChild(label.name);
			parent.appendChild(label.value);
		};

		var content = document.createElement('div');

		if(this.hasLegend()) {
			this.element_legend_childs = {
				title_date: document.createElement('span'),
				title_time: document.createElement('span'),
				title_units: document.createElement('span'),
				nano: document.createElement('div'),
				nano_options: {
					paneClass: 'netdata-legend-series-pane',
					sliderClass: 'netdata-legend-series-slider',
					contentClass: 'netdata-legend-series-content',
					enabledClass: '__enabled',
					flashedClass: '__flashed',
					activeClass: '__active',
					tabIndex: -1,
					alwaysVisible: true
				},
				series: {}
			};

			this.element_legend.innerHTML = '';

			this.element_legend_childs.title_date.className += " netdata-legend-title-date";
			this.element_legend.appendChild(this.element_legend_childs.title_date);

			this.element_legend.appendChild(document.createElement('br'));

			this.element_legend_childs.title_time.className += " netdata-legend-title-time";
			this.element_legend.appendChild(this.element_legend_childs.title_time);

			this.element_legend.appendChild(document.createElement('br'));

			this.element_legend_childs.title_units.className += " netdata-legend-title-units";
			this.element_legend.appendChild(this.element_legend_childs.title_units);

			this.element_legend.appendChild(document.createElement('br'));

			this.element_legend_childs.nano.className = 'netdata-legend-series';
			this.element_legend.appendChild(this.element_legend_childs.nano);

			content.className = 'netdata-legend-series-content';
			this.element_legend_childs.nano.appendChild(content);
		}
		else {
			this.element_legend_childs = {
				title_date: null,
				title_time: null,
				title_units: null,
				nano: null,
				nano_options: null,
				series: {}
			};
		}

		if(this.current.data) {
			var me = this;
			$.each(me.current.data.dimension_names, function(i, d) {
				genLabel(me, content, d, i);
			});
		}
		else {
			var me = this;
			$.each(me.chart.dimensions, function(i, d) {
				genLabel(me, content, d.name, i);
			});
		}

		// create a hidden div to be used for hidding
		// the original legend of the chart library
		var el = document.createElement('div');
		this.element_legend.appendChild(el);
		el.style.display = 'none';

		this.element_legend_childs.hidden = document.createElement('div');
		el.appendChild(this.element_legend_childs.hidden);

		if(this.element_legend_childs.nano !== null && this.element_legend_childs.nano_options !== null)
			$(this.element_legend_childs.nano).nanoScroller(this.element_legend_childs.nano_options);

		this.legendShowLatestValues();
	}

	chartState.prototype.createChartDOM = function() {
		if(this.debug) this.log('creating DOM');

		this.element_chart_id = this.library_name + '-' + this.uuid + '-chart';
		this.element_chart = document.createElement('div');
		this.element_chart.className += ' netdata-chart' + (this.hasLegend()?'-with-legend-right':'').toString();
		this.element_chart.className += ' netdata-' + this.library_name + '-chart' + (this.hasLegend()?'-with-legend-right':'').toString();
		this.element_chart.id = this.element_chart_id;
		$(this.element_chart).data('netdata-state-object', this);
		this.element.appendChild(this.element_chart);

		this.element_legend_id = this.library_name + '-' + this.uuid + '-legend';
		this.element_legend = document.createElement('div');
		this.element_legend.className += ' netdata-chart-legend';
		this.element_legend.className += ' netdata-' + this.library_name + '-legend';
		this.element_legend.id = this.element_legend_id;
		$(this.element_legend).data('netdata-state-object', this);
		if(!this.hasLegend()) this.element_legend.style.display = 'none';
		else this.element.appendChild(this.element_legend);

		this.element_legend_childs.series = null;
		this.legendUpdateDOM();

		// in case the user has an active global selection sync in place
		// reset it
		this.globalSelectionSyncStop();
	}

	chartState.prototype.hasLegend = function() {
		if(typeof this.___hasLegendCache___ !== 'undefined')
			return this.___hasLegendCache___;

		var leg = false;
		if(this.library && this.library.legend(this) === 'right-side') {
			var legend = $(this.element).data('legend') || 'yes';
			if(legend === 'yes') leg = true;
		}

		this.___hasLegendCache___ = leg;
		return leg;
	}

	chartState.prototype.legendWidth = function() {
		return (this.hasLegend())?110:0;
	}

	chartState.prototype.legendHeight = function() {
		return $(this.element).height();
	}

	chartState.prototype.chartWidth = function() {
		return $(this.element).width() - this.legendWidth();
	}

	chartState.prototype.chartHeight = function() {
		return $(this.element).height();
	}

	chartState.prototype.chartPixelsPerPoint = function() {
		// force an options provided detail
		var px = this.pixels_per_point;

		if(this.library && px < this.library.pixels_per_point(this))
			px = this.library.pixels_per_point(this);

		if(px < NETDATA.options.current.pixels_per_point)
			px = NETDATA.options.current.pixels_per_point;

		return px;
	}

	chartState.prototype.needsResize = function() {
		return (this.library && !this.library.autoresize() && this.tm.last_resized < NETDATA.options.last_resized);
	}

	chartState.prototype.resizeChart = function() {
		if(this.needsResize()) {
			if(this.debug) this.log('forcing re-generation due to window resize.');
			this.tm.last_created = 0;
			this.tm.last_resized = new Date().getTime();
		}
	}

	chartState.prototype.chartURL = function() {
		var before;
		var after;
		if(NETDATA.globalPanAndZoom.isActive()) {
			after = Math.round(NETDATA.globalPanAndZoom.force_after_ms / 1000);
			before = Math.round(NETDATA.globalPanAndZoom.force_before_ms / 1000);
			this.tm.pan_and_zoom_seq = NETDATA.globalPanAndZoom.seq;
		}
		else {
			before = this.current.force_before_ms !== null ? Math.round(this.current.force_before_ms / 1000) : this.before;
			after  = this.current.force_after_ms  !== null ? Math.round(this.current.force_after_ms / 1000) : this.after;
			this.tm.pan_and_zoom_seq = 0;
		}

		this.current.requested_after_ms = after * 1000;
		this.current.requested_before_ms = before * 1000;

		this.current.points = this.points || Math.round(this.chartWidth() / this.chartPixelsPerPoint());

		// build the data URL
		this.current.url = this.chart.data_url;
		this.current.url += "&format="  + this.library.format();
		this.current.url += "&points="  + this.current.points.toString();
		this.current.url += "&group="   + this.method;
		this.current.url += "&options=" + this.library.options();
		this.current.url += '|jsonwrap';

		if(after)
			this.current.url += "&after="  + after.toString();

		if(before)
			this.current.url += "&before=" + before.toString();

		if(this.dimensions)
			this.current.url += "&dimensions=" + this.dimensions;

		if(NETDATA.options.debug.chart_data_url || this.debug) this.log('chartURL(): ' + this.current.url + ' WxH:' + this.chartWidth() + 'x' + this.chartHeight() + ' points: ' + this.current.points + ' library: ' + this.library_name);
	}

	chartState.prototype.updateChartWithData = function(data) {
		if(this.debug) this.log('got data from netdata server');
		this.current.data = data;

		var started = this.tm.last_updated = new Date().getTime();

		// if the result is JSON, find the latest update-every
		if(typeof data === 'object') {
			if(typeof data.view_update_every !== 'undefined')
				this.current.view_update_every = data.view_update_every * 1000;

			if(typeof data.after !== 'undefined')
				this.current.after_ms = data.after * 1000;

			if(typeof data.before !== 'undefined')
				this.current.before_ms = data.before * 1000;

			if(typeof data.first_entry_t !== 'undefined')
				this.current.first_entry_ms = data.first_entry_t * 1000;

			if(typeof data.last_entry_t !== 'undefined')
				this.current.last_entry_ms = data.last_entry_t * 1000;

			if(typeof data.points !== 'undefined')
				this.current.points = data.points;

			data.state = this;
		}

		this.updates_counter++;

		if(this.debug) {
			this.log('UPDATE No ' + this.updates_counter + ' COMPLETED');

			if(this.current.force_after_ms)
				this.log('STATUS: forced   : ' + (this.current.force_after_ms / 1000).toString() + ' - ' + (this.current.force_before_ms / 1000).toString());
			else
				this.log('STATUS: forced: unset');

			this.log('STATUS: requested: ' + (this.current.requested_after_ms / 1000).toString() + ' - ' + (this.current.requested_before_ms / 1000).toString());
			this.log('STATUS: rendered : ' + (this.current.after_ms / 1000).toString() + ' - ' + (this.current.before_ms / 1000).toString());
			this.log('STATUS: points   : ' + (this.current.points).toString() + ', min step: ' + (this._minPanOrZoomStep() / 1000).toString());
		}

		// this may force the chart to be re-created
		this.resizeChart();

		if(this.updates_since_last_creation >= this.library.max_updates_to_recreate()) {
			if(this.debug) this.log('max updates of ' + this.updates_since_last_creation.toString() + ' reached. Forcing re-generation.');
			this.tm.last_created = 0;
		}

		if(this.tm.last_created > 0 && typeof this.library.update === 'function') {
			if(this.debug) this.log('updating chart...');

			// check and update the legend
			this.legendUpdateDOM();

			this.updates_since_last_creation++;
			if(NETDATA.options.debug.chart_errors) {
				this.library.update(this, data);
			}
			else {
				try {
					this.library.update(this, data);
				}
				catch(err) {
					this.error('chart failed to be updated as ' + this.library_name);
				}
			}
		}
		else {
			if(this.debug) this.log('creating chart...');

			this.createChartDOM();
			this.updates_since_last_creation = 0;

			if(NETDATA.options.debug.chart_errors) {
				this.library.create(this, data);
				this.tm.last_created = new Date().getTime();
			}
			else {
				try {
					this.library.create(this, data);
					this.tm.last_created = new Date().getTime();
				}
				catch(err) {
					this.error('chart failed to be created as ' + this.library_name);
				}
			}
		}
		this.legendShowLatestValues();

		// update the performance counters
		var now = new Date().getTime();

		// don't update last_autorefreshed if this chart is
		// forced to be updated with global PanAndZoom
		if(NETDATA.globalPanAndZoom.isActive())
			this.current.last_autorefreshed = 0;
		else
			this.current.last_autorefreshed = now;

		this.refresh_dt_ms = now - started;
		NETDATA.options.auto_refresher_fast_weight += this.refresh_dt_ms;

		if(this.refresh_dt_element)
			this.refresh_dt_element.innerHTML = this.refresh_dt_ms.toString();
	}

	chartState.prototype.updateChart = function(callback) {
		// due to late initialization of charts and libraries
		// we need to check this too
		if(this.enabled === false) {
			if(this.debug) this.log('I am not enabled');
			if(typeof callback === 'function') callback();
			return false;
		}

		if(this.chart === null) {
			var self = this;
			this.getChart(function() { self.updateChart(callback); });
			return;
		}

		if(this.library.initialized === false) {
			var self = this;
			this.library.initialize(function() { self.updateChart(callback); });
			return;
		}

		this.clearSelection();
		this.chartURL();
		this.showLoading();
		if(this.debug) this.log('updating from ' + this.current.url);

		var self = this;
		this.xhr = $.ajax( {
			url: this.current.url,
			crossDomain: NETDATA.options.crossDomainAjax,
			cache: false,
			async: true
		})
		.success(function(data) {
			self.hideLoading();
			if(self.debug) self.log('data received. updating chart.');
			self.updateChartWithData(data);
		})
		.fail(function() {
			self.hideLoading();
			self.error('data download failed for url: ' + self.current.url);
		})
		.always(function() {
			self.hideLoading();
			if(typeof callback === 'function') callback();
		});
	}

	chartState.prototype.destroyChart = function() {
		this.current.last_autorefreshed = new Date().getTime();

		if(this.element_message !== null) {
			this.element_message.innerHTML = '';
			this.element_message = null;
		}

		if(this.element_loading !== null) {
			this.element_loading.innerHTML = '';
			this.element_loading = null;
		}

		if(this.element_legend !== null) {
			this.element_legend.innerHTML = '';
			this.element_legend = null;
		}

		if(this.element_chart !== null) {
			this.element_chart.innerHTML = '';
			this.element_chart = null;
		}

		this.element_legend_childs = {
			hidden: null,
			title_date: null,
			title_time: null,
			title_units: null,
			nano: null,
			nano_options: null,
			series: null
		};

		this.element.innerHTML = '';
		this.refresh_dt_element = null;

		this.tm.last_created = 0;
		this.paused = false;
		this.selected = false;
		this.updates_counter = 0;
		this.updates_since_last_creation = 0;
		this.tm.pan_and_zoom_seq = 0;
		this.tm.last_resized = 0;
		this.tm.last_visible_check = 0;
		this.tm.last_hidden = 0;
		this.tm.last_unhidden = 0;

		if(this.current !== null) {
			this.current.last_autorefreshed = 0;
			this.current.view_update_every = 0;
			this.current.after_ms = 0;
			this.current.before_ms = 0;
			this.current.points = 0;
			this.current.data = null;
			this.current.force_update_at = 0;
			this.current.force_after_ms = null;
			this.current.force_before_ms = null;
			this.current.requested_after_ms = null;
			this.current.requested_before_ms = null;
			this.current.first_entry_ms = null;
			this.current.last_entry_ms = null;
		}
		this.init();
	}

	chartState.prototype.unhideChart = function() {
		if(typeof this.___isHidden___ !== 'undefined' && this.enabled === true) {
			this.element_message.style.display = 'none';
			if(this.element_chart !== null) this.element_chart.style.display = 'inline-block';
			if(this.element_legend !== null) this.element_legend.style.display = 'inline-block';
			if(this.element_loading !== null) this.element_loading.style.display = 'none';
			this.___isHidden___ = undefined;
			this.element_message.innerHTML = 'chart ' + this.id + ' is visible now';

			// refresh the scrolbar
			if(this.element_legend_childs.nano !== null && this.element_legend_childs.nano_options !== null)
				$(this.element_legend_childs.nano).nanoScroller(this.element_legend_childs.nano_options);

			this.tm.last_unhidden = new Date().getTime();
		}
	}

	chartState.prototype.hideChart = function() {
		if(typeof this.___isHidden___ === 'undefined' && this.enabled === true) {
			if(NETDATA.options.current.destroy_on_hide === true)
				this.destroyChart();

			this.element_message.style.display = 'inline-block';
			if(this.element_chart !== null) this.element_chart.style.display = 'none';
			if(this.element_legend !== null) this.element_legend.style.display = 'none';
			if(this.element_loading !== null) this.element_loading.style.display = 'none';
			this.___isHidden___ = true;
			this.___showsLoading___ = undefined;
			this.element_message.innerHTML = 'chart ' + this.id + ' is hidden to speed up the browser';
			this.tm.last_hidden = new Date().getTime();
		}
	}

	chartState.prototype.hideLoading = function() {
		if(typeof this.___showsLoading___ !== 'undefined' && this.enabled === true) {
			this.element_message.style.display = 'none';
			if(this.element_chart !== null) this.element_chart.style.display = 'inline-block';
			if(this.element_legend !== null) this.element_legend.style.display = 'inline-block';
			if(this.element_loading !== null) this.element_loading.style.display = 'none';
			this.___showsLoading___ = undefined;
			this.element_loading.innerHTML = 'chart ' + this.id + ' finished loading!';
			this.tm.last_unhidden = new Date().getTime();
		}
	}

	chartState.prototype.showLoading = function() {
		if(typeof this.___showsLoading___ === 'undefined' && this.tm.last_created === 0 && this.enabled === true) {
			this.element_message.style.display = 'none';
			if(this.element_chart !== null) this.element_chart.style.display = 'none';
			if(this.element_legend !== null) this.element_legend.style.display = 'none';
			if(this.element_loading !== null) this.element_loading.style.display = 'inline-block';
			this.___showsLoading___ = true;
			this.___isHidden___ = undefined;
			this.element_loading.innerHTML = 'chart ' + this.id + ' is loading...';
			this.tm.last_hidden = new Date().getTime();
		}
	}

	chartState.prototype.isVisible = function() {
		// this.log('last_visible_check: ' + this.tm.last_visible_check + ', last_page_scroll: ' + NETDATA.options.last_page_scroll);
		if(this.tm.last_visible_check > NETDATA.options.last_page_scroll)
			return this.___isVisible___;

		this.tm.last_visible_check = new Date().getTime();

		var wh = window.innerHeight;
		var x = this.element.getBoundingClientRect();
		var ret = 0;
		var tolerance = 0;

		if(x.top < 0 && -x.top > x.height) {
			// the chart is entirely above
			ret = -x.top - x.height;
		}
		else if(x.top > wh) {
			// the chart is entirely below
			ret = x.top - wh;
		}

		if(ret > tolerance) {
			// the chart is too far
			this.___isVisible___ = false;
			if(this.tm.last_created !== 0) this.hideChart();
			return this.___isVisible___;
		}
		else {
			// the chart is inside or very close
			this.___isVisible___ = true;
			this.unhideChart();
			return this.___isVisible___;
		}
	}

	chartState.prototype.isAutoRefreshed = function() {
		return (this.current.autorefresh);
	}

	chartState.prototype.canBeAutoRefreshed = function() {
		now = new Date().getTime();

		if(this.enabled === false) {
			if(this.debug) this.log('I am not enabled');
			return false;
		}

		if(this.library === null || this.library.enabled === false) {
			this.error('charting library "' + this.library_name + '" is not available');
			if(this.debug) this.log('My chart library ' + this.library_name + ' is not available');
			return false;
		}

		if(this.isVisible() === false) {
			if(NETDATA.options.debug.visibility || this.debug) this.log('I am not visible');
			return;
		}
		
		if(this.current.force_update_at !== 0 && this.current.force_update_at < now) {
			if(this.debug) this.log('timed force update detecting - allowing this update');
			this.current.force_update_at = 0;
			return true;
		}

		if(this.isAutoRefreshed()) {
			// allow the first update, even if the page is not visible
			if(this.updates_counter && !NETDATA.options.page_is_visible) {
				if(NETDATA.options.debug.focus || this.debug) this.log('canBeAutoRefreshed(): page does not have focus');
				return false;
			}

			// options valid only for autoRefresh()
			if(NETDATA.options.auto_refresher_stop_until === 0 || NETDATA.options.auto_refresher_stop_until < now) {
				if(NETDATA.globalPanAndZoom.isActive()) {
					if(NETDATA.globalPanAndZoom.shouldBeAutoRefreshed(this)) {
						if(this.debug) this.log('canBeAutoRefreshed(): global panning: I need an update.');
						return true;
					}
					else {
						if(this.debug) this.log('canBeAutoRefreshed(): global panning: I am already up to date.');
						return false;
					}
				}

				if(this.selected) {
					if(this.debug) this.log('canBeAutoRefreshed(): I have a selection in place.');
					return false;
				}

				if(this.paused) {
					if(this.debug) this.log('canBeAutoRefreshed(): I am paused.');
					return false;
				}

				if(now - this.current.last_autorefreshed > this.current.view_update_every) {
					if(this.debug) this.log('canBeAutoRefreshed(): It is time to update me.');
					return true;
				}
			}
		}

		return false;
	}

	chartState.prototype.autoRefresh = function(callback) {
		if(this.canBeAutoRefreshed()) {
			this.updateChart(callback);
		}
		else {
			if(typeof callback !== 'undefined')
				callback();
		}
	}

	chartState.prototype._defaultsFromDownloadedChart = function(chart) {
		this.chart = chart;
		this.chart_url = chart.url;
		this.current.view_update_every = chart.update_every * 1000;
		this.current.points = Math.round(this.chartWidth() / this.chartPixelsPerPoint());
		this.tm.last_info_downloaded = new Date().getTime();
	}

	// fetch the chart description from the netdata server
	chartState.prototype.getChart = function(callback) {
		this.chart = NETDATA.chartRegistry.get(this.host, this.id);
		if(this.chart) {
			this._defaultsFromDownloadedChart(this.chart);
			if(typeof callback === 'function') callback();
		}
		else {
			this.chart_url = this.host + "/api/v1/chart?chart=" + this.id;
			if(this.debug) this.log('downloading ' + this.chart_url);
			var self = this;

			$.ajax( {
				url:  this.chart_url,
				crossDomain: NETDATA.options.crossDomainAjax,
				cache: false,
				async: true
			})
			.done(function(chart) {
				chart.url = self.chart_url;
				chart.data_url = (self.host + chart.data_url);
				self._defaultsFromDownloadedChart(chart);
				NETDATA.chartRegistry.add(self.host, self.id, chart);
			})
			.fail(function() {
				NETDATA.error(404, self.chart_url);
				self.error('chart not found on url "' + self.chart_url + '"');
			})
			.always(function() {
				if(typeof callback === 'function') callback();
			});
		}
	}

	// resize the chart to its real dimensions
	// as given by the caller
	chartState.prototype.sizeChart = function() {
		this.element.className += " netdata-container";

		if(this.debug) this.log('sizing element');

		if(this.width)
			$(this.element).css('width', this.width);

		if(this.height)
			$(this.element).css('height', this.height);

		if(NETDATA.chartDefaults.min_width)
			$(this.element).css('min-width', NETDATA.chartDefaults.min_width);
	}

	// show a message in the chart
	chartState.prototype.message = function(type, msg) {
		this.hideChart();
		this.element_message.innerHTML = msg;

		// reset the creation datetime
		// since we overwrote the whole element
		if(this.debug) this.log(msg);
	}

	// show an error on the chart and stop it forever
	chartState.prototype.error = function(msg) {
		this.message('error', this.id + ': ' + msg);
		this.enabled = false;
	}

	// show a message indicating the chart is loading
	chartState.prototype.info = function(msg) {
		this.message('info', this.id + ': ' + msg);
	}

	chartState.prototype.init = function() {
		this.element.innerHTML = '';

		this.element_message = document.createElement('div');
		this.element_message.className += ' netdata-message';
		this.element.appendChild(this.element_message);

		this.element_loading = document.createElement('div');
		this.element_loading.className += ' netdata-chart-is-loading';
		this.element.appendChild(this.element_loading);

		if(this.debug) this.log('created');
		this.sizeChart();

		// make sure the host does not end with /
		// all netdata API requests use absolute paths
		while(this.host.slice(-1) === '/')
			this.host = this.host.substring(0, this.host.length - 1);

		// check the requested library is available
		// we don't initialize it here - it will be initialized when
		// this chart will be first used
		if(typeof NETDATA.chartLibraries[this.library_name] === 'undefined') {
			NETDATA.error(402, this.library_name);
			this.error('chart library "' + this.library_name + '" is not found');
		}
		else if(!NETDATA.chartLibraries[this.library_name].enabled) {
			NETDATA.error(403, this.library_name);
			this.error('chart library "' + this.library_name + '" is not enabled');
		}
		else
			this.library = NETDATA.chartLibraries[this.library_name];

		// if we need to report the rendering speed
		// find the element that needs to be updated
		if(this.refresh_dt_element_name)
			this.refresh_dt_element = document.getElementById(this.refresh_dt_element_name) || null;

		// the default mode for all charts
		this.setMode('auto');
	}

	// get or create a chart state, given a DOM element
	NETDATA.chartState = function(element) {
		var state = $(element).data('netdata-state-object') || null;
		if(!state) {
			state = new chartState(element);
			$(element).data('netdata-state-object', state);
		}
		return state;
	}

	// ----------------------------------------------------------------------------------------------------------------
	// Library functions

	// Load a script without jquery
	// This is used to load jquery - after it is loaded, we use jquery
	NETDATA._loadjQuery = function(callback) {
		if(typeof jQuery === 'undefined') {
			if(NETDATA.options.debug.main_loop) console.log('loading ' + NETDATA.jQuery);

			var script = document.createElement('script');
			script.type = 'text/javascript';
			script.async = true;
			script.src = NETDATA.jQuery;

			// script.onabort = onError;
			script.onerror = function(err, t) { NETDATA.error(101, NETDATA.jQuery); };
			if(typeof callback === "function")
				script.onload = callback;

			var s = document.getElementsByTagName('script')[0];
			s.parentNode.insertBefore(script, s);
		}
		else if(typeof callback === "function")
			callback();
	}

	NETDATA._loadCSS = function(filename) {
		// don't use jQuery here
		// styles are loaded before jQuery
		// to eliminate showing an unstyled page to the user
		
		var fileref = document.createElement("link");
		fileref.setAttribute("rel", "stylesheet");
		fileref.setAttribute("type", "text/css");
		fileref.setAttribute("href", filename);

		if (typeof fileref !== 'undefined')
			document.getElementsByTagName("head")[0].appendChild(fileref);
	}

	NETDATA.colorHex2Rgb = function(hex) {
		// Expand shorthand form (e.g. "03F") to full form (e.g. "0033FF")
		var shorthandRegex = /^#?([a-f\d])([a-f\d])([a-f\d])$/i;
			hex = hex.replace(shorthandRegex, function(m, r, g, b) {
			return r + r + g + g + b + b;
		});

		var result = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex);
		return result ? {
			r: parseInt(result[1], 16),
			g: parseInt(result[2], 16),
			b: parseInt(result[3], 16)
		} : null;
	}

	NETDATA.colorLuminance = function(hex, lum) {
		// validate hex string
		hex = String(hex).replace(/[^0-9a-f]/gi, '');
		if (hex.length < 6)
			hex = hex[0]+hex[0]+hex[1]+hex[1]+hex[2]+hex[2];

		lum = lum || 0;

		// convert to decimal and change luminosity
		var rgb = "#", c, i;
		for (i = 0; i < 3; i++) {
			c = parseInt(hex.substr(i*2,2), 16);
			c = Math.round(Math.min(Math.max(0, c + (c * lum)), 255)).toString(16);
			rgb += ("00"+c).substr(c.length);
		}

		return rgb;
	}

	NETDATA.guid = function() {
		function s4() {
			return Math.floor((1 + Math.random()) * 0x10000)
					.toString(16)
					.substring(1);
			}

			return s4() + s4() + '-' + s4() + '-' + s4() + '-' + s4() + '-' + s4() + s4() + s4();
	}

	NETDATA.zeropad = function(x) {
		if(x > -10 && x < 10) return '0' + x.toString();
		else return x.toString();
	}

	// user function to signal us the DOM has been
	// updated.
	NETDATA.updatedDom = function() {
		NETDATA.options.updated_dom = true;
	}

	NETDATA.ready = function(callback) {
		NETDATA.options.readyCallback = callback;
	}

	NETDATA.pause = function(callback) {
		NETDATA.options.pauseCallback = callback;
	}

	NETDATA.unpause = function() {
		NETDATA.options.pauseCallback = null;
		NETDATA.options.updated_dom = true;
		NETDATA.options.pause = false;
	}

	// ----------------------------------------------------------------------------------------------------------------

	NETDATA.chartRefresherNoParallel = function(index) {
		if(NETDATA.options.debug.mail_loop) console.log('NETDATA.chartRefresherNoParallel(' + index + ')');

		if(NETDATA.options.updated_dom) {
			// the dom has been updated
			// get the dom parts again
			NETDATA.parseDom(NETDATA.chartRefresher);
			return;
		}
		if(index >= NETDATA.options.targets.length) {
			if(NETDATA.options.debug.main_loop) console.log('waiting to restart main loop...');
				NETDATA.options.auto_refresher_fast_weight = 0;

				setTimeout(function() {
					NETDATA.chartRefresher();
				}, NETDATA.options.current.idle_between_loops);
			}
		else {
			var state = NETDATA.options.targets[index];

			if(NETDATA.options.auto_refresher_fast_weight < NETDATA.options.current.fast_render_timeframe) {
				if(NETDATA.options.debug.main_loop) console.log('fast rendering...');

				state.autoRefresh(function() {
					NETDATA.chartRefresherNoParallel(++index);
				});
			}
			else {
				if(NETDATA.options.debug.main_loop) console.log('waiting for next refresh...');
				NETDATA.options.auto_refresher_fast_weight = 0;

				setTimeout(function() {
					state.autoRefresh(function() {
						NETDATA.chartRefresherNoParallel(++index);
					});
				}, NETDATA.options.current.idle_between_charts);
			}
		}
	}

	NETDATA.chartRefresher_sequencial = function() {
		if(NETDATA.options.updated_dom) {
			// the dom has been updated
			// get the dom parts again
			NETDATA.parseDom(NETDATA.chartRefresher);
			return;
		}
		
		if(NETDATA.options.sequencial.length === 0)
			NETDATA.chartRefresher();
		else {
			var state = NETDATA.options.sequencial.pop();
			if(state.library.initialized)
				NETDATA.chartRefresher();
			else
				state.autoRefresh(NETDATA.chartRefresher_sequencial);
		}
	}

	NETDATA.chartRefresher = function() {
		if(NETDATA.options.pause) {
			// console.log('auto-refresher is paused');
			setTimeout(NETDATA.chartRefresher,
				NETDATA.options.current.idle_between_loops);
			return;
		}

		if(typeof NETDATA.options.pauseCallback === 'function') {
			// console.log('auto-refresher is calling pauseCallback');
			NETDATA.options.pause = true;
			NETDATA.options.pauseCallback();
			NETDATA.chartRefresher();
		}

		if(!NETDATA.options.current.parallel_refresher) {
			NETDATA.chartRefresherNoParallel(0);
			return;
		}

		if(NETDATA.options.updated_dom) {
			// the dom has been updated
			// get the dom parts again
			NETDATA.parseDom(NETDATA.chartRefresher);
			return;
		}

		NETDATA.options.parallel = new Array();
		NETDATA.options.sequencial = new Array();

		var targets = NETDATA.options.targets;
		var len = targets.length;
		while(len--) {
			var state = targets[len];

			if(!state.library.initialized)
				NETDATA.options.sequencial.push(state);
			else
				NETDATA.options.parallel.push(state);
		}

		if(NETDATA.options.parallel.length > 0) {
			NETDATA.options.parallel_jobs = NETDATA.options.parallel.length;

			$(NETDATA.options.parallel).each(function() {
				this.autoRefresh(function() {
					NETDATA.options.parallel_jobs--;
					if(NETDATA.options.parallel_jobs === 0) {
						setTimeout(NETDATA.chartRefresher_sequencial,
							NETDATA.options.current.idle_between_charts);
					}
				});
			})
		}
		else {
			setTimeout(NETDATA.chartRefresher_sequencial,
				NETDATA.options.current.idle_between_charts);
		}
	}

	NETDATA.parseDom = function(callback) {
		NETDATA.options.last_page_scroll = new Date().getTime();
		NETDATA.options.updated_dom = false;

		var targets = $('div[data-netdata]').filter(':visible');

		if(NETDATA.options.debug.main_loop)
			console.log('DOM updated - there are ' + targets.length + ' charts on page.');

		NETDATA.options.targets = new Array();
		var len = targets.length;
		while(len--) {
			// the initialization will take care of sizing
			// and the "loading..." message
			NETDATA.options.targets.push(NETDATA.chartState(targets[len]));
		}

		if(typeof callback === 'function') callback();
	}

	// this is the main function - where everything starts
	NETDATA.start = function() {
		// this should be called only once

		NETDATA.options.page_is_visible = true;

		$(window).blur(function() {
			NETDATA.options.page_is_visible = false;
			if(NETDATA.options.debug.focus) console.log('Lost Focus!');
		});

		$(window).focus(function() {
			NETDATA.options.page_is_visible = true;
			if(NETDATA.options.debug.focus) console.log('Focus restored!');
		});

		if(typeof document.hasFocus === 'function' && !document.hasFocus()) {
			NETDATA.options.page_is_visible = false;
			if(NETDATA.options.debug.focus) console.log('Document has no focus!');
		}

		NETDATA.parseDom(NETDATA.chartRefresher);
	}

	// ----------------------------------------------------------------------------------------------------------------
	// peity

	NETDATA.peityInitialize = function(callback) {
		if(typeof netdataNoPeitys === 'undefined' || !netdataNoPeitys) {
			$.ajax({
				url: NETDATA.peity_js,
				cache: true,
				dataType: "script"
			})
				.done(function() {
					NETDATA.registerChartLibrary('peity', NETDATA.peity_js);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.peity_js);
				})
				.always(function() {
					if(typeof callback === "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.peity.enabled = false;
			if(typeof callback === "function")
				callback();
		}
	};

	NETDATA.peityChartUpdate = function(state, data) {
		$(state.peity_instance).html(data.result);
		// $(state.element_chart).change() does not accept options
		// to pass width and height
		//$(state.peity_instance).change();
		$(state.peity_instance).peity('line', { width: state.chartWidth(), height: state.chartHeight() });
	}

	NETDATA.peityChartCreate = function(state, data) {
		state.peity_instance = document.createElement('div');
		state.element_chart.appendChild(state.peity_instance);

		$(state.peity_instance).html(data.result);
		$(state.peity_instance).peity('line', { width: state.chartWidth(), height: state.chartHeight() });
	}

	// ----------------------------------------------------------------------------------------------------------------
	// sparkline

	NETDATA.sparklineInitialize = function(callback) {
		if(typeof netdataNoSparklines === 'undefined' || !netdataNoSparklines) {
			$.ajax({
				url: NETDATA.sparkline_js,
				cache: true,
				dataType: "script"
			})
				.done(function() {
					NETDATA.registerChartLibrary('sparkline', NETDATA.sparkline_js);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.sparkline_js);
				})
				.always(function() {
					if(typeof callback === "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.sparkline.enabled = false;
			if(typeof callback === "function") 
				callback();
		}
	};

	NETDATA.sparklineChartUpdate = function(state, data) {
		state.sparkline_options.width = state.chartWidth();
		state.sparkline_options.height = state.chartHeight();

		$(state.element_chart).sparkline(data.result, state.sparkline_options);
	}

	NETDATA.sparklineChartCreate = function(state, data) {
		var self = $(state.element);
		var type = self.data('sparkline-type') || 'line';
		var lineColor = self.data('sparkline-linecolor') || state.chartColors()[0];
		var fillColor = self.data('sparkline-fillcolor') || (state.chart.chart_type === 'line')?'#FFF':NETDATA.colorLuminance(lineColor, NETDATA.chartDefaults.fill_luminance);
		var chartRangeMin = self.data('sparkline-chartrangemin') || undefined;
		var chartRangeMax = self.data('sparkline-chartrangemax') || undefined;
		var composite = self.data('sparkline-composite') || undefined;
		var enableTagOptions = self.data('sparkline-enabletagoptions') || undefined;
		var tagOptionPrefix = self.data('sparkline-tagoptionprefix') || undefined;
		var tagValuesAttribute = self.data('sparkline-tagvaluesattribute') || undefined;
		var disableHiddenCheck = self.data('sparkline-disablehiddencheck') || undefined;
		var defaultPixelsPerValue = self.data('sparkline-defaultpixelspervalue') || undefined;
		var spotColor = self.data('sparkline-spotcolor') || undefined;
		var minSpotColor = self.data('sparkline-minspotcolor') || undefined;
		var maxSpotColor = self.data('sparkline-maxspotcolor') || undefined;
		var spotRadius = self.data('sparkline-spotradius') || undefined;
		var valueSpots = self.data('sparkline-valuespots') || undefined;
		var highlightSpotColor = self.data('sparkline-highlightspotcolor') || undefined;
		var highlightLineColor = self.data('sparkline-highlightlinecolor') || undefined;
		var lineWidth = self.data('sparkline-linewidth') || undefined;
		var normalRangeMin = self.data('sparkline-normalrangemin') || undefined;
		var normalRangeMax = self.data('sparkline-normalrangemax') || undefined;
		var drawNormalOnTop = self.data('sparkline-drawnormalontop') || undefined;
		var xvalues = self.data('sparkline-xvalues') || undefined;
		var chartRangeClip = self.data('sparkline-chartrangeclip') || undefined;
		var xvalues = self.data('sparkline-xvalues') || undefined;
		var chartRangeMinX = self.data('sparkline-chartrangeminx') || undefined;
		var chartRangeMaxX = self.data('sparkline-chartrangemaxx') || undefined;
		var disableInteraction = self.data('sparkline-disableinteraction') || false;
		var disableTooltips = self.data('sparkline-disabletooltips') || false;
		var disableHighlight = self.data('sparkline-disablehighlight') || false;
		var highlightLighten = self.data('sparkline-highlightlighten') || 1.4;
		var highlightColor = self.data('sparkline-highlightcolor') || undefined;
		var tooltipContainer = self.data('sparkline-tooltipcontainer') || undefined;
		var tooltipClassname = self.data('sparkline-tooltipclassname') || undefined;
		var tooltipFormat = self.data('sparkline-tooltipformat') || undefined;
		var tooltipPrefix = self.data('sparkline-tooltipprefix') || undefined;
		var tooltipSuffix = self.data('sparkline-tooltipsuffix') || ' ' + state.chart.units;
		var tooltipSkipNull = self.data('sparkline-tooltipskipnull') || true;
		var tooltipValueLookups = self.data('sparkline-tooltipvaluelookups') || undefined;
		var tooltipFormatFieldlist = self.data('sparkline-tooltipformatfieldlist') || undefined;
		var tooltipFormatFieldlistKey = self.data('sparkline-tooltipformatfieldlistkey') || undefined;
		var numberFormatter = self.data('sparkline-numberformatter') || function(n){ return n.toFixed(2); };
		var numberDigitGroupSep = self.data('sparkline-numberdigitgroupsep') || undefined;
		var numberDecimalMark = self.data('sparkline-numberdecimalmark') || undefined;
		var numberDigitGroupCount = self.data('sparkline-numberdigitgroupcount') || undefined;
		var animatedZooms = self.data('sparkline-animatedzooms') || false;

		state.sparkline_options = {
			type: type,
			lineColor: lineColor,
			fillColor: fillColor,
			chartRangeMin: chartRangeMin,
			chartRangeMax: chartRangeMax,
			composite: composite,
			enableTagOptions: enableTagOptions,
			tagOptionPrefix: tagOptionPrefix,
			tagValuesAttribute: tagValuesAttribute,
			disableHiddenCheck: disableHiddenCheck,
			defaultPixelsPerValue: defaultPixelsPerValue,
			spotColor: spotColor,
			minSpotColor: minSpotColor,
			maxSpotColor: maxSpotColor,
			spotRadius: spotRadius,
			valueSpots: valueSpots,
			highlightSpotColor: highlightSpotColor,
			highlightLineColor: highlightLineColor,
			lineWidth: lineWidth,
			normalRangeMin: normalRangeMin,
			normalRangeMax: normalRangeMax,
			drawNormalOnTop: drawNormalOnTop,
			xvalues: xvalues,
			chartRangeClip: chartRangeClip,
			chartRangeMinX: chartRangeMinX,
			chartRangeMaxX: chartRangeMaxX,
			disableInteraction: disableInteraction,
			disableTooltips: disableTooltips,
			disableHighlight: disableHighlight,
			highlightLighten: highlightLighten,
			highlightColor: highlightColor,
			tooltipContainer: tooltipContainer,
			tooltipClassname: tooltipClassname,
			tooltipChartTitle: state.chart.title,
			tooltipFormat: tooltipFormat,
			tooltipPrefix: tooltipPrefix,
			tooltipSuffix: tooltipSuffix,
			tooltipSkipNull: tooltipSkipNull,
			tooltipValueLookups: tooltipValueLookups,
			tooltipFormatFieldlist: tooltipFormatFieldlist,
			tooltipFormatFieldlistKey: tooltipFormatFieldlistKey,
			numberFormatter: numberFormatter,
			numberDigitGroupSep: numberDigitGroupSep,
			numberDecimalMark: numberDecimalMark,
			numberDigitGroupCount: numberDigitGroupCount,
			animatedZooms: animatedZooms,
			width: state.chartWidth(),
			height: state.chartHeight()
		};

		$(state.element_chart).sparkline(data.result, state.sparkline_options);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// dygraph

	NETDATA.dygraph = {
		smooth: false
	};

	NETDATA.dygraphSetSelection = function(state, t) {
		if(typeof state.dygraph_instance !== 'undefined') {
			var r = state.calculateRowForTime(t);
			if(r !== -1)
				state.dygraph_instance.setSelection(r);
			else {
				state.dygraph_instance.clearSelection();
				state.legendUndefined();
			}
		}

		return true;
	}

	NETDATA.dygraphClearSelection = function(state, t) {
		if(typeof state.dygraph_instance !== 'undefined') {
			state.dygraph_instance.clearSelection();
		}
		return true;
	}

	NETDATA.dygraphSmoothInitialize = function(callback) {
		$.ajax({
			url: NETDATA.dygraph_smooth_js,
			cache: true,
			dataType: "script"
		})
			.done(function() {
				NETDATA.dygraph.smooth = true;
				smoothPlotter.smoothing = 0.3;
			})
			.always(function() {
				if(typeof callback === "function")
					callback();
			})
	};

	NETDATA.dygraphInitialize = function(callback) {
		if(typeof netdataNoDygraphs === 'undefined' || !netdataNoDygraphs) {
			$.ajax({
				url: NETDATA.dygraph_js,
				cache: true,
				dataType: "script"
			})
				.done(function() {
					NETDATA.registerChartLibrary('dygraph', NETDATA.dygraph_js);
					NETDATA.dygraphSmoothInitialize(callback);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.dygraph_js);
					if(typeof callback === "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.dygraph.enabled = false;
			if(typeof callback === "function")
				callback();
		}
	};

	NETDATA.dygraphChartUpdate = function(state, data) {
		var dygraph = state.dygraph_instance;
		
		// when the chart is not visible, and hidden
		// if there is a window resize, dygraph detects
		// its element size as 0x0.
		// this will make it re-appear properly

		if(state.tm.last_unhidden > state.dygraph_last_rendered)
			dygraph.resize();

		if(state.current.name === 'pan') {
			if(NETDATA.options.debug.dygraph || state.debug) state.log('dygraphChartUpdate() loose update');
			dygraph.updateOptions({
				file: data.result.data,
				labels: data.result.labels,
				labelsDivWidth: state.chartWidth() - 70
			});
		}
		else {
			if(NETDATA.options.debug.dygraph || state.debug) state.log('dygraphChartUpdate() strict update');
			dygraph.updateOptions({
				file: data.result.data,
				labels: data.result.labels,
				labelsDivWidth: state.chartWidth() - 70,
				dateWindow: null,
    			valueRange: null
			});
		}

		state.dygraph_last_rendered = new Date().getTime();
	};

	NETDATA.dygraphChartCreate = function(state, data) {
		if(NETDATA.options.debug.dygraph || state.debug) state.log('dygraphChartCreate()');

		var self = $(state.element);

		var chart_type = state.chart.chart_type;
		if(chart_type === 'stacked' && data.dimensions === 1) chart_type = 'area';
		chart_type = self.data('dygraph-type') || chart_type;

		var smooth = (chart_type === 'line' && !NETDATA.chartLibraries.dygraph.isSparkline(state))?true:false;
		smooth = self.data('dygraph-smooth') || smooth;

		if(!NETDATA.dygraph.smooth)
			smooth = false;

		var strokeWidth = (chart_type === 'stacked')?0.0:((smooth)?1.5:1.0)
		var highlightCircleSize = (NETDATA.chartLibraries.dygraph.isSparkline(state))?3:4;


		//if(state.id == 'system.cpu')
		//	console.log(data);

		state.dygraph_options = {
			colors: self.data('dygraph-colors') || state.chartColors(),
			
			// leave a few pixels empty on the right of the chart
			rightGap: self.data('dygraph-rightgap') || 5,
			showRangeSelector: self.data('dygraph-showrangeselector') || false,
			showRoller: self.data('dygraph-showroller') || false,

			title: self.data('dygraph-title') || state.chart.title,
			titleHeight: self.data('dygraph-titleheight') || 19,

//			legend: self.data('dygraph-legend') || NETDATA.chartLibraries.dygraph.legend(state)?'always':'never', // 'onmouseover',
			legend: self.data('dygraph-legend') || 'always', // 'onmouseover',
			labels: data.result.labels,
			labelsDiv: self.data('dygraph-labelsdiv') || state.element_legend_childs.hidden,
			labelsDivStyles: self.data('dygraph-labelsdivstyles') || { 'fontSize':'10px', 'zIndex': 10000 },
			labelsDivWidth: self.data('dygraph-labelsdivwidth') || state.chartWidth() - 70,
			labelsSeparateLines: self.data('dygraph-labelsseparatelines') || true,
			labelsShowZeroValues: self.data('dygraph-labelsshowzerovalues') || true,
			labelsKMB: false,
			labelsKMG2: false,
			showLabelsOnHighlight: self.data('dygraph-showlabelsonhighlight') || true,
			hideOverlayOnMouseOut: self.data('dygraph-hideoverlayonmouseout') || true,

			ylabel: state.chart.units,
			yLabelWidth: self.data('dygraph-ylabelwidth') || 12,

			// the function to plot the chart
			plotter: null,

			// The width of the lines connecting data points. This can be used to increase the contrast or some graphs.
			strokeWidth: self.data('dygraph-strokewidth') || strokeWidth,
			strokePattern: self.data('dygraph-strokepattern') || undefined,

			// The size of the dot to draw on each point in pixels (see drawPoints). A dot is always drawn when a point is "isolated",
			// i.e. there is a missing point on either side of it. This also controls the size of those dots.
			drawPoints: self.data('dygraph-drawpoints') || false,
			
			// Draw points at the edges of gaps in the data. This improves visibility of small data segments or other data irregularities.
			drawGapEdgePoints: self.data('dygraph-drawgapedgepoints') || true,

			connectSeparatedPoints: self.data('dygraph-connectseparatedpoints') || false,
			pointSize: self.data('dygraph-pointsize') || 1,

			// enabling this makes the chart with little square lines
			stepPlot: self.data('dygraph-stepplot') || false,
			
			// Draw a border around graph lines to make crossing lines more easily distinguishable. Useful for graphs with many lines.
			strokeBorderColor: self.data('dygraph-strokebordercolor') || 'white',
			strokeBorderWidth: self.data('dygraph-strokeborderwidth') || (chart_type === 'stacked')?0.0:0.0,

			fillGraph: self.data('dygraph-fillgraph') || (chart_type === 'area')?true:false,
			fillAlpha: self.data('dygraph-fillalpha') || (chart_type === 'stacked')?0.8:0.2,
			stackedGraph: self.data('dygraph-stackedgraph') || (chart_type === 'stacked')?true:false,
			stackedGraphNaNFill: self.data('dygraph-stackedgraphnanfill') || 'none',
			
			drawAxis: self.data('dygraph-drawaxis') || true,
			axisLabelFontSize: self.data('dygraph-axislabelfontsize') || 10,
			axisLineColor: self.data('dygraph-axislinecolor') || '#CCC',
			axisLineWidth: self.data('dygraph-axislinewidth') || 0.3,

			drawGrid: self.data('dygraph-drawgrid') || true,
			drawXGrid: self.data('dygraph-drawxgrid') || undefined,
			drawYGrid: self.data('dygraph-drawygrid') || undefined,
			gridLinePattern: self.data('dygraph-gridlinepattern') || null,
			gridLineWidth: self.data('dygraph-gridlinewidth') || 0.3,
			gridLineColor: self.data('dygraph-gridlinecolor') || '#DDD',

			maxNumberWidth: self.data('dygraph-maxnumberwidth') || 8,
			sigFigs: self.data('dygraph-sigfigs') || null,
			digitsAfterDecimal: self.data('dygraph-digitsafterdecimal') || 2,
			valueFormatter: self.data('dygraph-valueformatter') || function(x){ return x.toFixed(2); },

			highlightCircleSize: self.data('dygraph-highlightcirclesize') || highlightCircleSize,
			highlightSeriesOpts: self.data('dygraph-highlightseriesopts') || null, // TOO SLOW: { strokeWidth: 1.5 },
			highlightSeriesBackgroundAlpha: self.data('dygraph-highlightseriesbackgroundalpha') || null, // TOO SLOW: (chart_type === 'stacked')?0.7:0.5,

			pointClickCallback: self.data('dygraph-pointclickcallback') || undefined,
			axes: {
				x: {
					pixelsPerLabel: 50,
					ticker: Dygraph.dateTicker,
					axisLabelFormatter: function (d, gran) {
						return NETDATA.zeropad(d.getHours()) + ":" + NETDATA.zeropad(d.getMinutes()) + ":" + NETDATA.zeropad(d.getSeconds());
					},
					valueFormatter: function (ms) {
						var d = new Date(ms);
						return d.toLocaleDateString() + ' ' + d.toLocaleTimeString();
						// return NETDATA.zeropad(d.getHours()) + ":" + NETDATA.zeropad(d.getMinutes()) + ":" + NETDATA.zeropad(d.getSeconds());
					}
				},
				y: {
					pixelsPerLabel: 15,
					valueFormatter: function (x) {
						// return (Math.round(x*100) / 100).toLocaleString();
						//return state.legendFormatValue(x);
						//FIXME
						return x;
					}
				}
			},
			legendFormatter: function(data) {
				var g = data.dygraph;
				var html;
				var elements = state.element_legend_childs;

				// if the hidden div is not there
				// we are not managing the legend
				if(elements.hidden === null) return;

				if (typeof data.x === 'undefined') {
					state.legendReset();
				}
				else {
					state.legendSetDate(data.x);
					var i = data.series.length;
					while(i--) {
						var series = data.series[i];
						if(!series.isVisible) continue;
						state.legendSetLabelValue(series.label, series.y);
					}
				}

				return '';
			},
			drawCallback: function(dygraph, is_initial) {
				if(state.current.name !== 'auto') {
					if(NETDATA.options.debug.dygraph) state.log('dygraphDrawCallback()');

					var x_range = dygraph.xAxisRange();
					var after = Math.round(x_range[0]);
					var before = Math.round(x_range[1]);

					state.updateChartPanOrZoom(after, before);
				}
			},
			zoomCallback: function(minDate, maxDate, yRanges) {
				if(NETDATA.options.debug.dygraph) state.log('dygraphZoomCallback()');
				state.globalSelectionSyncStop();
				state.globalSelectionSyncDelay();
				state.updateChartPanOrZoom(minDate, maxDate);
			},
			highlightCallback: function(event, x, points, row, seriesName) {
				if(NETDATA.options.debug.dygraph || state.debug) state.log('dygraphHighlightCallback()');
				state.pauseChart();

				// there is a bug in dygraph when the chart is zoomed enough
				// the time it thinks is selected is wrong
				// here we calculate the time t based on the row number selected
				// which is ok
				var t = state.current.after_ms + row * state.current.view_update_every;
				// console.log('row = ' + row + ', x = ' + x + ', t = ' + t + ' ' + ((t === x)?'SAME':'DIFFERENT') + ', rows in db: ' + state.current.data.points + ' visible(x) = ' + state.timeIsVisible(x) + ' visible(t) = ' + state.timeIsVisible(t) + ' r(x) = ' + state.calculateRowForTime(x) + ' r(t) = ' + state.calculateRowForTime(t) + ' range: ' + state.current.after_ms + ' - ' + state.current.before_ms + ' real: ' + state.current.data.after + ' - ' + state.current.data.before + ' every: ' + state.current.view_update_every);

				state.globalSelectionSync(t);

				// fix legend zIndex using the internal structures of dygraph legend module
				// this works, but it is a hack!
				// state.dygraph_instance.plugins_[0].plugin.legend_div_.style.zIndex = 10000;
			},
			unhighlightCallback: function(event) {
				if(NETDATA.options.debug.dygraph || state.debug) state.log('dygraphUnhighlightCallback()');
				state.unpauseChart();
				state.globalSelectionSyncStop();
			},
			interactionModel : {
				mousedown: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.mousedown()');
					state.globalSelectionSyncStop();

					if(NETDATA.options.debug.dygraph) state.log('dygraphMouseDown()');

					// Right-click should not initiate a zoom.
					if(event.button && event.button === 2) return;

					context.initializeMouseDown(event, dygraph, context);
					
					if(event.button && event.button === 1) {
						if (event.altKey || event.shiftKey) {
							state.setMode('pan');
							state.globalSelectionSyncDelay();
							Dygraph.startPan(event, dygraph, context);
						}
						else {
							state.setMode('zoom');
							state.globalSelectionSyncDelay();
							Dygraph.startZoom(event, dygraph, context);
						}
					}
					else {
						if (event.altKey || event.shiftKey) {
							state.setMode('zoom');
							state.globalSelectionSyncDelay();
							Dygraph.startZoom(event, dygraph, context);
						}
						else {
							state.setMode('pan');
							state.globalSelectionSyncDelay();
							Dygraph.startPan(event, dygraph, context);
						}
					}
				},
				mousemove: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.mousemove()');

					if(context.isPanning) {
						state.globalSelectionSyncStop();
						state.globalSelectionSyncDelay();
						state.setMode('pan');
						Dygraph.movePan(event, dygraph, context);
					}
					else if(context.isZooming) {
						state.globalSelectionSyncStop();
						state.globalSelectionSyncDelay();
						state.setMode('zoom');
						Dygraph.moveZoom(event, dygraph, context);
					}
				},
				mouseup: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.mouseup()');

					if (context.isPanning) {
						state.globalSelectionSyncDelay();
						Dygraph.endPan(event, dygraph, context);
					}
					else if (context.isZooming) {
						state.globalSelectionSyncDelay();
						Dygraph.endZoom(event, dygraph, context);
					}
				},
				click: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.click()');
					/*Dygraph.cancelEvent(event);*/
				},
				dblclick: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.dblclick()');
					state.globalSelectionSyncStop();
					NETDATA.globalPanAndZoom.clearMaster();
					state.resetChart();
				},
				mousewheel: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.mousewheel()');

					if(event.altKey || event.shiftKey) {
						state.globalSelectionSyncStop();
						state.globalSelectionSyncDelay();

						// http://dygraphs.com/gallery/interaction-api.js
						var normal = (event.detail) ? event.detail * -1 : event.wheelDelta / 40;
						var percentage = normal / 25;

						var before_old = state.current.before_ms;
						var after_old = state.current.after_ms;
						var range_old = before_old - after_old;

						var range = range_old * ( 1 - percentage );
						var dt = Math.round((range_old - range) / 2);

						var before = before_old - dt;
						var after  = after_old  + dt;

						if(NETDATA.options.debug.dygraph) state.log('percent: ' + percentage + ' from ' + after_old + ' - ' + before_old + ' to ' + after + ' - ' + before + ', range from ' + (before_old - after_old).toString() + ' to ' + (before - after).toString());

						state.setMode('zoom');
						state.updateChartPanOrZoom(after, before);
					}					
				},
				touchstart: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.touchstart()');
					state.globalSelectionSyncStop();
					state.globalSelectionSyncDelay();
					Dygraph.Interaction.startTouch(event, dygraph, context);
					context.touchDirections = { x: true, y: false };
					state.setMode('zoom');
				},
				touchmove: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.touchmove()');
					//Dygraph.cancelEvent(event);
					state.globalSelectionSyncStop();
					Dygraph.Interaction.moveTouch(event, dygraph, context);
				},
				touchend: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph || state.debug) state.log('interactionModel.touchend()');
					Dygraph.Interaction.endTouch(event, dygraph, context);
				}
			}
		};

		if(NETDATA.chartLibraries.dygraph.isSparkline(state)) {
			state.dygraph_options.drawGrid = false;
			state.dygraph_options.drawAxis = false;
			state.dygraph_options.title = undefined;
			state.dygraph_options.units = undefined;
			state.dygraph_options.ylabel = undefined;
			state.dygraph_options.yLabelWidth = 0;
			state.dygraph_options.labelsDivWidth = 120;
			state.dygraph_options.labelsDivStyles.width = '120px';
			state.dygraph_options.labelsSeparateLines = true;
			state.dygraph_options.rightGap = 0;
		}
		
		if(smooth === true && typeof smoothPlotter === 'function')
			state.dygraph_options.plotter = smoothPlotter;

		state.dygraph_instance = new Dygraph(state.element_chart,
			data.result.data, state.dygraph_options);

		state.dygraph_last_rendered = new Date().getTime();
	};

	// ----------------------------------------------------------------------------------------------------------------
	// morris

	NETDATA.morrisInitialize = function(callback) {
		if(typeof netdataNoMorris === 'undefined' || !netdataNoMorris) {

			// morris requires raphael
			if(!NETDATA.chartLibraries.raphael.initialized) {
				if(NETDATA.chartLibraries.raphael.enabled) {
					NETDATA.raphaelInitialize(function() {
						NETDATA.morrisInitialize(callback);
					});
				}
				else {
					NETDATA.chartLibraries.morris.enabled = false;
					if(typeof callback === "function")
						callback();
				}
			}
			else {
				NETDATA._loadCSS(NETDATA.morris_css);

				$.ajax({
					url: NETDATA.morris_js,
					cache: true,
					dataType: "script"
				})
					.done(function() {
						NETDATA.registerChartLibrary('morris', NETDATA.morris_js);
					})
					.fail(function() {
						NETDATA.error(100, NETDATA.morris_js);
					})
					.always(function() {
						if(typeof callback === "function")
							callback();
					})
			}
		}
		else {
			NETDATA.chartLibraries.morris.enabled = false;
			if(typeof callback === "function")
				callback();
		}
	};

	NETDATA.morrisChartUpdate = function(state, data) {
		state.morris_instance.setData(data.result.data);
	};

	NETDATA.morrisChartCreate = function(state, data) {

		state.morris_options = {
				element: state.element_chart_id,
				data: data.result.data,
				xkey: 'time',
				ykeys: data.dimension_names,
				labels: data.dimension_names,
				lineWidth: 2,
				pointSize: 3,
				smooth: true,
				hideHover: 'auto',
				parseTime: true,
				continuousLine: false,
				behaveLikeLine: false
		};

		if(state.chart.chart_type === 'line')
			state.morris_instance = new Morris.Line(state.morris_options);

		else if(state.chart.chart_type === 'area') {
			state.morris_options.behaveLikeLine = true;
			state.morris_instance = new Morris.Area(state.morris_options);
		}
		else // stacked
			state.morris_instance = new Morris.Area(state.morris_options);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// raphael

	NETDATA.raphaelInitialize = function(callback) {
		if(typeof netdataStopRaphael === 'undefined') {
			$.ajax({
				url: NETDATA.raphael_js,
				cache: true,
				dataType: "script"
			})
				.done(function() {
					NETDATA.registerChartLibrary('raphael', NETDATA.raphael_js);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.raphael_js);
				})
				.always(function() {
					if(typeof callback === "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.raphael.enabled = false;
			if(typeof callback === "function")
				callback();
		}
	};

	NETDATA.raphaelChartUpdate = function(state, data) {
		$(state.element_chart).raphael(data.result, {
			width: state.chartWidth(),
			height: state.chartHeight()
		})
	};

	NETDATA.raphaelChartCreate = function(state, data) {
		$(state.element_chart).raphael(data.result, {
			width: state.chartWidth(),
			height: state.chartHeight()
		})
	};

	// ----------------------------------------------------------------------------------------------------------------
	// google charts

	NETDATA.googleInitialize = function(callback) {
		if(typeof netdataNoGoogleCharts === 'undefined' || !netdataNoGoogleCharts) {
			$.ajax({
				url: NETDATA.google_js,
				cache: true,
				dataType: "script"
			})
				.done(function() {
					NETDATA.registerChartLibrary('google', NETDATA.google_js);

					google.load('visualization', '1.1', {
						'packages': ['corechart', 'controls'],
						'callback': callback
					});
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.google_js);
					if(typeof callback === "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.google.enabled = false;
			if(typeof callback === "function")
				callback();
		}
	};

	NETDATA.googleChartUpdate = function(state, data) {
		var datatable = new google.visualization.DataTable(data.result);
		state.google_instance.draw(datatable, state.google_options);
	};

	NETDATA.googleChartCreate = function(state, data) {
		var datatable = new google.visualization.DataTable(data.result);

		state.google_options = {
			// do not set width, height - the chart resizes itself
			//width: state.chartWidth(),
			//height: state.chartHeight(),
			lineWidth: 1,
			title: state.chart.title,
			fontSize: 11,
			hAxis: {
			//	title: "Time of Day",
			//	format:'HH:mm:ss',
				viewWindowMode: 'maximized',
				slantedText: false,
				format:'HH:mm:ss',
				textStyle: {
					fontSize: 9
				},
				gridlines: {
					color: '#EEE'
				}
			},
			vAxis: {
				title: state.chart.units,
				viewWindowMode: 'pretty',
				minValue: -0.1,
				maxValue: 0.1,
				direction: 1,
				textStyle: {
					fontSize: 9
				},
				gridlines: {
					color: '#EEE'
				}
			},
			chartArea: {
				width: '65%',
				height: '80%'
			},
			focusTarget: 'category',
			annotation: {
				'1': {
					style: 'line'
				}
			},
			pointsVisible: 0,
			titlePosition: 'out',
			titleTextStyle: {
				fontSize: 11
			},
			tooltip: {
				isHtml: false,
				ignoreBounds: true,
				textStyle: {
					fontSize: 9
				}
			},
			curveType: 'function',
			areaOpacity: 0.3,
			isStacked: false
		};

		switch(state.chart.chart_type) {
			case "area":
				state.google_options.vAxis.viewWindowMode = 'maximized';
				state.google_instance = new google.visualization.AreaChart(state.element_chart);
				break;

			case "stacked":
				state.google_options.isStacked = true;
				state.google_options.areaOpacity = 0.85;
				state.google_options.vAxis.viewWindowMode = 'maximized';
				state.google_options.vAxis.minValue = null;
				state.google_options.vAxis.maxValue = null;
				state.google_instance = new google.visualization.AreaChart(state.element_chart);
				break;

			default:
			case "line":
				state.google_options.lineWidth = 2;
				state.google_instance = new google.visualization.LineChart(state.element_chart);
				break;
		}

		state.google_instance.draw(datatable, state.google_options);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// easy-pie-chart

	NETDATA.easypiechartInitialize = function(callback) {
		if(typeof netdataStopEasypiechart === 'undefined') {
			$.ajax({
				url: NETDATA.easypiechart_js,
				cache: true,
				dataType: "script"
			})
				.done(function() {
					NETDATA.registerChartLibrary('easypiechart', NETDATA.easypiechart_js);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.easypiechart_js);
				})
				.always(function() {
					if(typeof callback === "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.easypiechart.enabled = false;
			if(typeof callback === "function")
				callback();
		}
	};

	NETDATA.easypiechartChartUpdate = function(state, data) {

		state.easypiechart_instance.update();
	};

	NETDATA.easypiechartChartCreate = function(state, data) {
		var self = $(state.element);

		var value = 10;
		var pcent = 10;

		$(state.element_chart).data('data-percent', pcent);
		data.element_chart.innerHTML = value.toString();

		state.easypiechart_instance = new EasyPieChart(state.element_chart, {
			barColor: self.data('easypiechart-barcolor') || '#ef1e25',
			trackColor: self.data('easypiechart-trackcolor') || '#f2f2f2',
			scaleColor: self.data('easypiechart-scalecolor') || '#dfe0e0',
			scaleLength: self.data('easypiechart-scalelength') || 5,
			lineCap: self.data('easypiechart-linecap') || 'round',
			lineWidth: self.data('easypiechart-linewidth') || 3,
			trackWidth: self.data('easypiechart-trackwidth') || undefined,
			size: self.data('easypiechart-size') || Math.min(state.chartWidth(), state.chartHeight()),
			rotate: self.data('easypiechart-rotate') || 0,
			animate: self.data('easypiechart-rotate') || {duration: 0, enabled: false},
			easing: self.data('easypiechart-easing') || undefined
		})
	};

	// ----------------------------------------------------------------------------------------------------------------
	// Charts Libraries Registration

	NETDATA.chartLibraries = {
		"dygraph": {
			initialize: NETDATA.dygraphInitialize,
			create: NETDATA.dygraphChartCreate,
			update: NETDATA.dygraphChartUpdate,
			setSelection: NETDATA.dygraphSetSelection,
			clearSelection:  NETDATA.dygraphClearSelection,
			initialized: false,
			enabled: true,
			format: function(state) { return 'json'; },
			options: function(state) { return 'ms|flip'; },
			legend: function(state) {
				if(!this.isSparkline(state))
					return 'right-side';
				else
					return null;
			},
			autoresize: function(state) { return true; },
			max_updates_to_recreate: function(state) { return 5000; },
			pixels_per_point: function(state) {
				if(!this.isSparkline(state))
					return 3;
				else
					return 2;
			},

			isSparkline: function(state) {
				if(typeof state.dygraph_sparkline === 'undefined') {
					var t = $(state.element).data('dygraph-theme');
					if(t === 'sparkline')
						state.dygraph_sparkline = true;
					else
						state.dygraph_sparkline = false;
				}
				return state.dygraph_sparkline;
			}
		},
		"sparkline": {
			initialize: NETDATA.sparklineInitialize,
			create: NETDATA.sparklineChartCreate,
			update: NETDATA.sparklineChartUpdate,
			setSelection: function(t) { return true; },
			clearSelection: function() { return true; },
			initialized: false,
			enabled: true,
			format: function(state) { return 'array'; },
			options: function(state) { return 'flip|abs'; },
			legend: function(state) { return null; },
			autoresize: function(state) { return false; },
			max_updates_to_recreate: function(state) { return 5000; },
			pixels_per_point: function(state) { return 3; }
		},
		"peity": {
			initialize: NETDATA.peityInitialize,
			create: NETDATA.peityChartCreate,
			update: NETDATA.peityChartUpdate,
			setSelection: function(t) { return true; },
			clearSelection: function() { return true; },
			initialized: false,
			enabled: true,
			format: function(state) { return 'ssvcomma'; },
			options: function(state) { return 'null2zero|flip|abs'; },
			legend: function(state) { return null; },
			autoresize: function(state) { return false; },
			max_updates_to_recreate: function(state) { return 5000; },
			pixels_per_point: function(state) { return 3; }
		},
		"morris": {
			initialize: NETDATA.morrisInitialize,
			create: NETDATA.morrisChartCreate,
			update: NETDATA.morrisChartUpdate,
			setSelection: function(t) { return true; },
			clearSelection: function() { return true; },
			initialized: false,
			enabled: true,
			format: function(state) { return 'json'; },
			options: function(state) { return 'objectrows|ms'; },
			legend: function(state) { return null; },
			autoresize: function(state) { return false; },
			max_updates_to_recreate: function(state) { return 50; },
			pixels_per_point: function(state) { return 15; }
		},
		"google": {
			initialize: NETDATA.googleInitialize,
			create: NETDATA.googleChartCreate,
			update: NETDATA.googleChartUpdate,
			setSelection: function(t) { return true; },
			clearSelection: function() { return true; },
			initialized: false,
			enabled: true,
			format: function(state) { return 'datatable'; },
			options: function(state) { return ''; },
			legend: function(state) { return null; },
			autoresize: function(state) { return false; },
			max_updates_to_recreate: function(state) { return 300; },
			pixels_per_point: function(state) { return 4; }
		},
		"raphael": {
			initialize: NETDATA.raphaelInitialize,
			create: NETDATA.raphaelChartCreate,
			update: NETDATA.raphaelChartUpdate,
			setSelection: function(t) { return true; },
			clearSelection: function() { return true; },
			initialized: false,
			enabled: true,
			format: function(state) { return 'json'; },
			options: function(state) { return ''; },
			legend: function(state) { return null; },
			autoresize: function(state) { return false; },
			max_updates_to_recreate: function(state) { return 5000; },
			pixels_per_point: function(state) { return 3; }
		},
		"easypiechart": {
			initialize: NETDATA.easypiechartInitialize,
			create: NETDATA.easypiechartChartCreate,
			update: NETDATA.easypiechartChartUpdate,
			setSelection: function(t) { return true; },
			clearSelection: function() { return true; },
			initialized: false,
			enabled: true,
			format: function(state) { return 'json'; },
			options: function(state) { return ''; },
			legend: function(state) { return null; },
			autoresize: function(state) { return false; },
			max_updates_to_recreate: function(state) { return 5000; },
			pixels_per_point: function(state) { return 3; }
		}
	};

	NETDATA.registerChartLibrary = function(library, url) {
		if(NETDATA.options.debug.libraries)
			console.log("registering chart library: " + library);

		NETDATA.chartLibraries[library].url = url;
		NETDATA.chartLibraries[library].initialized = true;
		NETDATA.chartLibraries[library].enabled = true;
	}

	// ----------------------------------------------------------------------------------------------------------------
	// Start up

	NETDATA.requiredJs = [
		{
			url: NETDATA.serverDefault + 'lib/bootstrap.min.js',
			isAlreadyLoaded: function() {
				if(typeof $().emulateTransitionEnd == 'function')
					return true;
				else
					return false;
			}
		},
		{
			url: NETDATA.serverDefault + 'lib/jquery.nanoscroller.min.js',
			isAlreadyLoaded: function() { return false; }
		}
	];

	NETDATA.requiredCSS = [
		NETDATA.serverDefault + 'css/bootstrap.min.css',
		//NETDATA.serverDefault + 'css/bootstrap-theme.min.css',
		NETDATA.dashboard_css
	];

	NETDATA.loadRequiredJs = function(index, callback) {
		if(index >= NETDATA.requiredJs.length)  {
			if(typeof callback === 'function')
				callback();
			return;
		}

		if(NETDATA.requiredJs[index].isAlreadyLoaded()) {
			NETDATA.loadRequiredJs(++index, callback);
			return;
		}

		if(NETDATA.options.debug.main_loop) console.log('loading ' + NETDATA.requiredJs[index].url);
		$.ajax({
			url: NETDATA.requiredJs[index].url,
			cache: true,
			dataType: "script"
		})
		.success(function() {
			if(NETDATA.options.debug.main_loop) console.log('loaded ' + NETDATA.requiredJs[index].url);
			NETDATA.loadRequiredJs(++index, callback);
		})
		.fail(function() {
			alert('Cannot load required JS library: ' + NETDATA.requiredJs[index].url);
		})
	}

	NETDATA.loadRequiredCSS = function(index) {
		if(index >= NETDATA.requiredCSS.length)
			return;

		if(NETDATA.options.debug.main_loop) console.log('loading ' + NETDATA.requiredCSS[index]);
		NETDATA._loadCSS(NETDATA.requiredCSS[index]);
		NETDATA.loadRequiredCSS(++index);
	}

	NETDATA.errorReset();
	NETDATA.loadRequiredCSS(0);
	NETDATA._loadjQuery(function() {
		NETDATA.loadRequiredJs(0, function() {
			//NETDATA.chartRegistry.downloadAll(NETDATA.serverDefault, function() {
				// NETDATA._loadCSS(NETDATA.dashboard_css);
				if(typeof netdataDontStart === 'undefined' || !netdataDontStart) {
					if(NETDATA.options.debug.main_loop) console.log('starting chart refresh thread');
					NETDATA.start();
				}

				if(typeof NETDATA.options.readyCallback === 'function')
					NETDATA.options.readyCallback();
			//});
		});
	});

	// window.NETDATA = NETDATA;
// })(window, document);
