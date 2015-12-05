// You can set the following variables before loading this script:
//
// var netdataStopDygraph = 1;			// do not use dygraph
// var netdataStopSparkline = 1;		// do not use sparkline
// var netdataStopPeity = 1;			// do not use peity
// var netdataStopGoogleCharts = 1;		// do not use google
// var netdataStopMorris = 1;			// do not use morris
//
// You can also set the default netdata server, using the following.
// When this variable is not set, we assume the page is hosted on your
// netdata server already.
// var netdataServer = "http://yourhost:19999"; // set your NetData server

(function(window)
{
	// fix IE bug with console
	if(!window.console){ window.console = {log: function(){} }; }

	var NETDATA = window.NETDATA || {};

	// ----------------------------------------------------------------------------------------------------------------
	// Detect the netdata server

	// http://stackoverflow.com/questions/984510/what-is-my-script-src-url
	// http://stackoverflow.com/questions/6941533/get-protocol-domain-and-port-from-url
	NETDATA._scriptSource = function(scripts) {
		var script = null, base = null;

		if(typeof document.currentScript != 'undefined') {
			script = document.currentScript;
		}
		else {
			var all_scripts = document.getElementsByTagName('script');
			script = all_scripts[all_scripts.length - 1];
		}

		if (script.getAttribute.length != 'undefined')
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

	if(typeof netdataServer != 'undefined')
		NETDATA.serverDefault = netdataServer + "/";
	else
		NETDATA.serverDefault = NETDATA._scriptSource();

	NETDATA.jQuery       = NETDATA.serverDefault + 'lib/jquery-1.11.3.min.js';
	NETDATA.peity_js     = NETDATA.serverDefault + 'lib/jquery.peity.min.js';
	NETDATA.sparkline_js = NETDATA.serverDefault + 'lib/jquery.sparkline.min.js';
	NETDATA.dygraph_js   = NETDATA.serverDefault + 'lib/dygraph-combined.js';
	NETDATA.raphael_js   = NETDATA.serverDefault + 'lib/raphael-min.js';
	NETDATA.morris_js    = NETDATA.serverDefault + 'lib/morris.min.js';
	NETDATA.morris_css   = NETDATA.serverDefault + 'css/morris.css';
	NETDATA.google_js    = 'https://www.google.com/jsapi';
	NETDATA.colors	= [ '#3366CC', '#DC3912', '#FF9900', '#109618', '#990099', '#3B3EAC', '#0099C6',
						'#DD4477', '#66AA00', '#B82E2E', '#316395', '#994499', '#22AA99', '#AAAA11',
						'#6633CC', '#E67300', '#8B0707', '#329262', '#5574A6', '#3B3EAC' ];

	// ----------------------------------------------------------------------------------------------------------------
	// the defaults for all charts

	NETDATA.chartDefaults = {
		host: NETDATA.serverDefault,	// the server to get data from
		width: '100%',					// the chart width
		height: '100%',					// the chart height
		library: 'dygraph',				// the graphing library to use
		method: 'max',					// the grouping method
		before: 0,						// panning
		after: -600,					// panning
		pixels_per_point: 1				// the detail of the chart
	}

	NETDATA.options = {
		targets: null,				
		updated_dom: 1,
		auto_refresher_fast_weight: 0,
		page_is_visible: 1,
		double_click_ms: 100,
		auto_refresher_stop_until: 0,

		current: {
			pixels_per_point: 1,
			idle_between_charts: 50,
			idle_between_loops: 200,
			idle_lost_focus: 500,
			global_pan_sync_time: 500,
			fast_render_timeframe: 200 // render continously for these many ms
		},

		debug: {
			show_boxes: 		0,
			main_loop: 			0,
			focus: 				0,
			visibility: 		0,
			chart_data_url: 	0,
			chart_errors: 		0,
			chart_timing: 		0,
			chart_calls: 		0,
			dygraph: 			0
		}
	}

	if(NETDATA.options.debug.main_loop) console.log('welcome to NETDATA');


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

	NETDATA.chartRegistry = {
		charts: {},

		add: function(host, id, data) {
			host = host.replace(/:/g, "_").replace(/\//g, "_");
			id   =   id.replace(/:/g, "_").replace(/\//g, "_");

			if(typeof this.charts[host] == 'undefined')
				this.charts[host] = {};

			this.charts[host][id] = data;
		},

		get: function(host, id) {
			host = host.replace(/:/g, "_").replace(/\//g, "_");
			id   =   id.replace(/:/g, "_").replace(/\//g, "_");

			if(typeof this.charts[host] == 'undefined')
				return null;

			if(typeof this.charts[host][id] == 'undefined')
				return null;

			return this.charts[host][id];
		}
	};

	// ----------------------------------------------------------------------------------------------------------------

	NETDATA.globalPanAndZoom = {
		seq: 0,
		state: null,
		before_ms: null,
		after_ms: null,
		force_before_ms: null,
		force_after_ms: null,

		setMaster: function(state, after, before) {
			if(this.state && this.state != state) this.state.resetChart();

			this.state = state;
			this.seq = new Date().getTime();
			this.force_after_ms = after;
			this.force_before_ms = before;
		},
		clearMaster: function() {
			if(this.state) {
				var state = this.state;
				this.state = null; // prevent infinite recursion
				this.seq = 0;
				state.resetChart();
				NETDATA.options.auto_refresher_stop_until = 0;
			}
			else {
				this.state = null;
				this.seq = 0;
			}
		},
		shouldBeAutoRefreshed: function(state) {
			if(!this.state || !this.seq)
				return false;

			if(state.follows_global == this.seq)
				return false;

			return true;
		}
	}

	// ----------------------------------------------------------------------------------------------------------------
	// Our state object, where all per-chart values are stored

	NETDATA.chartInitialState = function(element) {
		self = $(element);

		var state = {
			uuid: NETDATA.guid(),	// GUID - a unique identifier for the chart
			id: self.data('netdata'),	// string - the name of chart

			// the user given dimensions of the element
			width: self.data('width') || NETDATA.chartDefaults.width,
			height: self.data('height') || NETDATA.chartDefaults.height,

			// these are calculated every time the chart is refreshed
			calculated_width: 0,
			calculated_height: 0,

			// string - the netdata server URL, without any path
			host: self.data('host') || NETDATA.chartDefaults.host,

			// string - the grouping method requested by the user
			method: self.data('method') || NETDATA.chartDefaults.method,

			// the time-range requested by the user
			after: self.data('after') || NETDATA.chartDefaults.after,
			before: self.data('before') || NETDATA.chartDefaults.before,

			// the pixels per point requested by the user
			pixels_per_point: 1,
			points: self.data('points') || null,

			// the dimensions requested by the user
			dimensions: self.data('dimensions') || null,

			// the chart library requested by the user
			library_name: self.data('chart-library') || NETDATA.chartDefaults.library,
			library: null,			// object - the chart library used

			element: element,		// the element it is linked to
			
			chart_url: null,		// string - the url to download chart info
			chart: null,			// object - the chart as downloaded from the server

			downloaded_ms: 0,		// milliseconds - the timestamp we downloaded the chart
			created_ms: 0,			// boolean - the timestamp the chart was created
			validated: false, 		// boolean - has the chart been validated?
			enabled: true, 			// boolean - is the chart enabled for refresh?
			paused: false,			// boolean - is the chart paused for any reason?
			debug: false,
			updates_counter: 0,		// numeric - the number of refreshes made so far

			follows_global: 0,

			mode: null, 			// auto, pan, zoom
			auto: {
				name: 'auto',
				autorefresh: true,
				url: 'invalid://',	// string - the last url used to update the chart
				last_updated_ms: 0, // milliseconds - the timestamp of last refresh
				view_update_every: 0, 	// milliseconds - the minimum acceptable refresh duration
				after_ms: 0,		// milliseconds - the first timestamp of the data
				before_ms: 0,		// milliseconds - the last timestamp of the data
				points: 0,			// number - the number of points in the data
				data: null,			// the last downloaded data
				force_before_ms: null,
				force_after_ms: null,
				requested_before_ms: null,
				requested_after_ms: null,
				first_entry_ms: null,
				last_entry_ms: null,
			},
			pan: {
				name: 'pan',
				autorefresh: false,
				url: 'invalid://',	// string - the last url used to update the chart
				last_updated_ms: 0, // milliseconds - the timestamp of last refresh
				view_update_every: 0, 	// milliseconds - the minimum acceptable refresh duration
				after_ms: 0,		// milliseconds - the first timestamp of the data
				before_ms: 0,		// milliseconds - the last timestamp of the data
				points: 0,			// number - the number of points in the data
				data: null,			// the last downloaded data
				force_before_ms: null,
				force_after_ms: null,
				requested_before_ms: null,
				requested_after_ms: null,
				first_entry_ms: null,
				last_entry_ms: null,
			},
			zoom: {
				name: 'zoom',
				autorefresh: false,
				url: 'invalid://',	// string - the last url used to update the chart
				last_updated_ms: 0, // milliseconds - the timestamp of last refresh
				view_update_every: 0, 	// milliseconds - the minimum acceptable refresh duration
				after_ms: 0,		// milliseconds - the first timestamp of the data
				before_ms: 0,		// milliseconds - the last timestamp of the data
				points: 0,			// number - the number of points in the data
				data: null,			// the last downloaded data
				force_before_ms: null,
				force_after_ms: null,
				requested_before_ms: null,
				requested_after_ms: null,
				first_entry_ms: null,
				last_entry_ms: null,
			},
			refresh_dt_ms: 0,		// milliseconds - the time the last refresh took
			refresh_dt_element_name: self.data('dt-element-name') || null,	// string - the element to print refresh_dt_ms
			refresh_dt_element: null,

			log(msg) {
				console.log(this.id + ' (' + this.library_name + ' ' + this.uuid + '): ' + msg);
			},

			setSelection: function(t) {
				if(typeof this.library.setSelection == 'function')
					return this.library.setSelection(this, t);
				else
					return false;
			},

			clearSelection: function() {
				if(typeof this.library.clearSelection == 'function')
					return this.library.clearSelection(this);
				else
					return false;
			},

			timeIsVisible: function(t) {
				if(t >= this.mode.after_ms && t <= this.mode.before_ms)
					return true;
				return false;
			},

			calculateRowForTime: function(t) {
				if(!this.timeIsVisible(t)) return -1;

				var r = Math.floor((t - this.mode.after_ms) / this.mode.view_update_every);
				// console.log(this.mode.data);

				return r;
			},

			pauseChart: function() {
				this.paused = true;
			},

			unpauseChart: function() {
				this.paused = false;
			},

			resetChart: function() {
				if(NETDATA.globalPanAndZoom.state == this)
					NETDATA.globalPanAndZoom.clearMaster();

				if(state.mode.name != 'auto')
					this.setMode('auto');

				this.mode.force_before_ms = null;
				this.mode.force_after_ms = null;
				this.mode.last_updated_ms = 0;
				this.follows_global = 0;
				this.paused = false;
				this.enabled = true;
				this.debug = false;

				state.updateChart();
			},

			setMode: function(m) {
				if(this.mode) {
					if(this.mode.name == m) return;

					this[m].url = this.mode.url;
					this[m].last_updated_ms = this.mode.last_updated_ms;
					this[m].view_update_every = this.mode.view_update_every;
					this[m].after_ms = this.mode.after_ms;
					this[m].before_ms = this.mode.before_ms;
					this[m].points = this.mode.points;
					this[m].data = this.mode.data;
					this[m].requested_before_ms = this.mode.requested_before_ms;
					this[m].requested_after_ms = this.mode.requested_after_ms;
					this[m].first_entry_ms = this.mode.first_entry_ms;
					this[m].last_entry_ms = this.mode.last_entry_ms;
				}

				if(m == 'auto')
					this.mode = this.auto;
				else if(m == 'pan')
					this.mode = this.pan;
				else if(m == 'zoom')
					this.mode = this.zoom;
				else
					this.mode = this.auto;

				this.mode.force_before_ms = null;
				this.mode.force_after_ms = null;

				if(this.debug) this.log('mode set to ' + this.mode.name);
			},

			_minPanOrZoomStep: function() {
				return (((this.mode.before_ms - this.mode.after_ms) / this.mode.points) * ((this.mode.points * 5 / 100) + 1) );
				// return this.mode.view_update_every * 10;
			},

			_shouldBeMoved: function(old_after, old_before, new_after, new_before) {
				var dt_after = Math.abs(old_after - new_after);
				var dt_before = Math.abs(old_before - new_before);
				var old_range = old_before - old_after;

				var new_range = new_before - new_after;
				var dt = Math.abs(old_range - new_range);
				var step = Math.max(dt_after, dt_before, dt);

				var min_step = this._minPanOrZoomStep();
				if(new_range < old_range && new_range / this.calculated_width < 100) {
					if(this.debug) this.log('_shouldBeMoved(' + (new_after / 1000).toString() + ' - ' + (new_before / 1000).toString() + '): minimum point size: 0.10, wanted point size: ' + (new_range / this.calculated_width / 1000).toString() + ': TOO SMALL RANGE');
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
			},

			updateChartPanOrZoom: function(after, before, callback) {
				var move = false;

				if(this.mode.name == 'auto') {
					if(this.debug) this.log('updateChartPanOrZoom(): caller did not set proper mode');
					this.setMode('pan');
				}
					
				if(!this.mode.force_after_ms || !this.mode.force_before_ms) {
					if(this.debug) this.log('updateChartPanOrZoom(' + (after / 1000).toString() + ' - ' + (before / 1000).toString() + '): INIT');
					move = true;
				}
				else if(this._shouldBeMoved(this.mode.force_after_ms, this.mode.force_before_ms, after, before) && this._shouldBeMoved(this.mode.after_ms, this.mode.before_ms, after, before)) {
					if(this.debug) this.log('updateChartPanOrZoom(' + (after / 1000).toString() + ' - ' + (before / 1000).toString() + '): FORCE CHANGE from ' + (this.mode.force_after_ms / 1000).toString() + ' - ' + (this.mode.force_before_ms / 1000).toString());
					move = true;
				}
				else if(this._shouldBeMoved(this.mode.requested_after_ms, this.mode.requested_before_ms, after, before) && this._shouldBeMoved(this.mode.after_ms, this.mode.before_ms, after, before)) {
					if(this.debug) this.log('updateChartPanOrZoom(' + (after / 1000).toString() + ' - ' + (before / 1000).toString() + '): REQUESTED CHANGE from ' + (this.mode.requested_after_ms / 1000).toString() + ' - ' + (this.mode.requested_before_ms / 1000).toString());
					move = true;
				}

				if(move) {
					var now = new Date().getTime();
					NETDATA.options.auto_refresher_stop_until = now + NETDATA.options.current.global_pan_sync_time;
					NETDATA.globalPanAndZoom.setMaster(this, after, before);

					this.mode.force_after_ms = after;
					this.mode.force_before_ms = before;
					this.updateChart(callback);
					return true;
				}

				if(this.debug) this.log('updateChartPanOrZoom(' + (after / 1000).toString() + ' - ' + (before / 1000).toString() + '): IGNORE');
				if(typeof callback != 'undefined') callback();
				return false;
			},

			updateChart: function(callback) {
				if(!this.library || !this.library.enabled) {
					this.error('chart library "' + this.library_name + '" is not enabled');
					if(typeof callback != 'undefined') callback();
					return;
				}

				if(!this.enabled) {
					if(this.debug) this.error('chart "' + this.id + '" is not enabled');
					if(typeof callback != 'undefined') callback();
					return;
				}

				if(!self.visible(true)) {
					if(NETDATA.options.debug.visibility || this.debug) this.log('is not visible');
					if(typeof callback != 'undefined') callback();
					return;
				}
				if(!this.chart) {
					var this_state_object = this;
					this.getChart(function() { this_state_object.updateChart(callback); });
					return;
				}

				if(!this.library.initialized) {
					var this_state_object = this;
					this.library.initialize(function() { this_state_object.updateChart(callback); });
					return;
				}
				
				this.clearSelection();
				this.chartURL();
				if(this.debug) this.log('updating from ' + this.mode.url);

				var this_state_object = this;
				$.ajax( {
					url: this_state_object.mode.url,
					crossDomain: true
				})
				.then(function(data) {
					if(this_state_object.debug) this_state_object.log('got data from netdata server');
					this_state_object.mode.data = data;

					var started = new Date().getTime();

					// if the result is JSON, find the latest update-every
					if(typeof data == 'object' && this_state_object.library.jsonWrapper) {
						if(!this_state_object.follows_global && typeof data.view_update_every != 'undefined')
							this_state_object.mode.view_update_every = data.view_update_every * 1000;

						if(typeof data.after != 'undefined')
							this_state_object.mode.after_ms = data.after * 1000;

						if(typeof data.before != 'undefined')
							this_state_object.mode.before_ms = data.before * 1000;

						if(typeof data.first_entry_t != 'undefined')
							this_state_object.mode.first_entry_ms = data.first_entry_t * 1000;

						if(typeof data.last_entry_t != 'undefined')
							this_state_object.mode.last_entry_ms = data.last_entry_t * 1000;

						if(typeof data.points != 'undefined')
							this_state_object.mode.points = data.points;

						data.state = this_state_object;
					}

					this_state_object.updates_counter++;

					if(this_state_object.debug) {
						this_state_object.log('UPDATE No ' + this_state_object.updates_counter + ' COMPLETED');

						if(this_state_object.mode.force_after_ms)
							this_state_object.log('STATUS: forced   : ' + (this_state_object.mode.force_after_ms / 1000).toString() + ' - ' + (this_state_object.mode.force_before_ms / 1000).toString());
						else
							this_state_object.log('STATUS: forced: unset');

						this_state_object.log('STATUS: requested: ' + (this_state_object.mode.requested_after_ms / 1000).toString() + ' - ' + (this_state_object.mode.requested_before_ms / 1000).toString());
						this_state_object.log('STATUS: rendered : ' + (this_state_object.mode.after_ms / 1000).toString() + ' - ' + (this_state_object.mode.before_ms / 1000).toString());
						this_state_object.log('STATUS: points   : ' + (this_state_object.mode.points).toString() + ', min step: ' + (this_state_object._minPanOrZoomStep() / 1000).toString());
					}

					if(this_state_object.created_ms) {
						if(this_state_object.debug) this_state_object.log('updating chart...');

						if(NETDATA.options.debug.chart_errors) {
							this_state_object.library.update(this_state_object.element, data);
						}
						else {
							try {
								this_state_object.library.update(this_state_object.element, data);
							}
							catch(err) {
								this_state_object.error('chart "' + state.id + '" failed to be updated as ' + state.library_name);
							}
						}
					}
					else {
						if(this_state_object.debug) this_state_object.log('creating chart...');

						if(NETDATA.options.debug.chart_errors) {
							this_state_object.library.create(this_state_object.element, data);
							this_state_object.created_ms = new Date().getTime();
						}
						else {
							try {
								this_state_object.library.create(this_state_object.element, data);
								this_state_object.created_ms = new Date().getTime();
							}
							catch(err) {
								this_state_object.error('chart "' + state.id + '" failed to be created as ' + state.library_name);
							}
						}
					}

					// update the performance counters
					this_state_object.mode.last_updated_ms = new Date().getTime();
					this_state_object.refresh_dt_ms = this_state_object.mode.last_updated_ms - started;
					NETDATA.options.auto_refresher_fast_weight += this_state_object.refresh_dt_ms;

					if(this_state_object.refresh_dt_element)
						this_state_object.refresh_dt_element.innerHTML = this_state_object.refresh_dt_ms.toString();
				})
				.fail(function() {
					this_state_object.error('cannot download chart from ' + this_state_object.mode.url);
				})
				.always(function() {
					if(typeof callback == 'function') callback();
				});
			},

			chartURL: function() {
				this.calculated_width = self.width();
				this.calculated_height = self.height();

				var before;
				var after;
				if(NETDATA.globalPanAndZoom.state) {
					after = Math.round(NETDATA.globalPanAndZoom.force_after_ms / 1000);
					before = Math.round(NETDATA.globalPanAndZoom.force_before_ms / 1000);
					this.follows_global = NETDATA.globalPanAndZoom.seq;
				}
				else {
					before = this.mode.force_before_ms != null ? Math.round(this.mode.force_before_ms / 1000) : this.before;
					after  = this.mode.force_after_ms  != null ? Math.round(this.mode.force_after_ms / 1000) : this.after;
					this.follows_global = 0;
				}

				this.mode.requested_after_ms = after * 1000;
				this.mode.requested_before_ms = before * 1000;

				// force an options provided detail
				var pixels_per_point = this.pixels_per_point;
				if(pixels_per_point < NETDATA.options.current.pixels_per_point)
					pixels_per_point = NETDATA.options.current.pixels_per_point

				this.mode.points = this.points || Math.round(this.calculated_width / pixels_per_point);

				// build the data URL
				this.mode.url = this.chart.data_url;
				this.mode.url += "&format="  + this.library.format;
				this.mode.url += "&points="  + this.mode.points.toString();
				this.mode.url += "&group="   + this.method;
				this.mode.url += "&options=" + this.library.options;
				if(this.library.jsonWrapper) this.mode.url += '|jsonwrap';

				if(after)
					this.mode.url += "&after="  + after.toString();

				if(before)
					this.mode.url += "&before=" + before.toString();

				if(this.dimensions)
					this.mode.url += "&dimensions=" + this.dimensions;

				if(NETDATA.options.debug.chart_data_url) this.log('chartURL(): ' + this.mode.url + ' WxH:' + this.calculated_width + 'x' + this.calculated_height + ' points: ' + this.mode.points + ' library: ' + this.library_name);
			},

			canBeAutoRefreshed: function(auto_refresher) {
				if(this.mode.autorefresh) {
					var now = new Date().getTime();

					if(this.updates_counter && !NETDATA.options.page_is_visible) {
						if(NETDATA.options.debug.focus || this.debug) this.log('page does not have focus');
						return false;
					}

					if(!auto_refresher) return true;

					if(!self.visible(true)) return false;

					// options valid only for autoRefresh()
					if(NETDATA.options.auto_refresher_stop_until == 0 || NETDATA.options.auto_refresher_stop_until < now) {
						if(NETDATA.globalPanAndZoom.state) {
							if(NETDATA.globalPanAndZoom.shouldBeAutoRefreshed(this))
								return true;
							else
								return false;
						}

						if(this.paused) return false;

						if(now - this.mode.last_updated_ms > this.mode.view_update_every)
							return true;
					}
				}

				return false;
			},

			autoRefresh: function(callback) {
				if(this.canBeAutoRefreshed(true))
					this.updateChart(callback);
				else if(typeof callback != 'undefined')
					callback();
			},

			// fetch the chart description from the netdata server
			getChart: function(callback) {
				this.chart = NETDATA.chartRegistry.get(this.host, this.id);
				if(this.chart) {
					if(typeof callback == 'function') callback();
				}
				else {
					this.chart_url = this.host + "/api/v1/chart?chart=" + this.id;
					if(this.debug) this.log('downloading ' + this.chart_url);
					this_state_object = this;

					$.ajax( {
						url:  this.chart_url,
						crossDomain: true
					})
					.done(function(chart) {
						chart.data_url = (this_state_object.host + chart.data_url);
						this_state_object.chart = chart;
						this_state_object.mode.view_update_every = chart.update_every * 1000;
						this_state_object.mode.points = Math.round(self.width() / (chart.update_every / 1000));

						chart.url = this_state_object.chart_url;
						NETDATA.chartRegistry.add(this_state_object.host, this_state_object.id, chart);
					})
					.fail(function() {
						NETDATA.error(404, this_state_object.chart_url);
						this_state_object.error('chart "' + this_state_object.id + '" not found on url "' + this_state_object.chart_url + '"');
					})
					.always(function() {
						if(typeof callback == 'function') callback();
					});
				}
			},

			// resize the chart to its real dimensions
			// as given by the caller
			sizeChart: function() {
				if(this.debug) this.log('sizing element');
				self.css('width', this.width)
					.css('height', this.height)
					.css('display', 'inline-block')
					.css('overflow', 'hidden');
			},

			// show a message in the chart
			message: function(msg) {
				var bgcolor = ""
				if(NETDATA.options.debug.show_boxes)
					bgcolor = " background-color: lightgrey;";

				this.element.innerHTML = '<div style="font-size: x-small; overflow: hidden;' + bgcolor + ' width: 100%; height: 100%;"><small>'
					+ msg
					+ '</small></div>';
				
				// reset the creation datetime
				// since we overwrote the whole element
				this.created_ms = 0
				if(this.debug) this.log(msg);
			},

			// show an error on the chart and stop it forever
			error: function(msg) {
				this.message(msg);
				this.enabled = false;
			},

			// show a message indicating the chart is loading
			loading: function() {
				this.message('chart ' + this.id + ' is loading...');
			}
		};

		if(state.debug) state.log('created');
		state.sizeChart();
		state.loading();

		if(typeof NETDATA.chartLibraries[state.library_name] == 'undefined') {
			NETDATA.error(402, state.library_name);
			state.error('chart library "' + state.library_name + '" is not found');
		}
		else if(!NETDATA.chartLibraries[state.library_name].enabled) {
			NETDATA.error(403, state.library_name);
			state.error('chart library "' + state.library_name + '" is not enabled');
		}
		else {
			state.library = NETDATA.chartLibraries[state.library_name];
			state.pixels_per_point = self.data('pixels-per-point') || state.library.pixels_per_point;
		}

		if(state.refresh_dt_element_name)
			state.refresh_dt_element = document.getElementById(state.refresh_dt_element_name) || null;

		state.setMode('auto');

		return state;
	}

	// get or create a chart state, given a DOM element
	NETDATA.chartState = function(element) {
		self = $(element);
		var state = self.data('state') || null;
		if(!state) {
			state = NETDATA.chartInitialState(element);
			self.data('state', state);
		}
		return state;
	}

	// ----------------------------------------------------------------------------------------------------------------
	// Library functions

	// Load a script without jquery
	// This is used to load jquery - after it is loaded, we use jquery
	NETDATA._loadjQuery = function(callback) {
		if(typeof jQuery == 'undefined') {
			var script = document.createElement('script');
			script.type = 'text/javascript';
			script.async = true;
			script.src = NETDATA.jQuery;

			// script.onabort = onError;
			script.onerror = function(err, t) { NETDATA.error(101, NETDATA.jQuery); };
			if(typeof callback == "function")
				script.onload = callback;

			var s = document.getElementsByTagName('script')[0];
			s.parentNode.insertBefore(script, s);
		}
		else if(typeof callback == "function")
			callback();
	}

	NETDATA.ColorLuminance = function(hex, lum) {
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

	// user function to signal us the DOM has been
	// updated.
	NETDATA.updatedDom = function() {
		NETDATA.options.updated_dom = 1;
	}

	// ----------------------------------------------------------------------------------------------------------------

	NETDATA.chartRefresher = function(index) {
		// if(NETDATA.options.debug.mail_loop) console.log('NETDATA.chartRefresher(<targets, ' + index + ')');

		if(NETDATA.options.updated_dom) {
			// the dom has been updated
			// get the dom parts again
			NETDATA.getDomCharts(function() {
				NETDATA.chartRefresher(0);
			});

			return;
		}

		var target = NETDATA.options.targets.get(index);
		if(target == null) {
			if(NETDATA.options.debug.main_loop) console.log('waiting to restart main loop...');
				NETDATA.options.auto_refresher_fast_weight = 0;

				setTimeout(function() {
					NETDATA.chartRefresher(0);
				}, NETDATA.options.current.idle_between_loops);
			}
		else {
			var state = NETDATA.chartState(target);

			if(NETDATA.options.auto_refresher_fast_weight < NETDATA.options.current.fast_render_timeframe) {
				if(NETDATA.options.debug.main_loop) console.log('fast rendering...');

				state.autoRefresh(function() {
					NETDATA.chartRefresher(++index);
				}, false);
			}
			else {
				if(NETDATA.options.debug.main_loop) console.log('waiting for next refresh...');
				NETDATA.options.auto_refresher_fast_weight = 0;

				setTimeout(function() {
					state.autoRefresh(function() {
						NETDATA.chartRefresher(++index);
					}, false);
				}, NETDATA.options.current.idle_between_charts);
			}
		}
	}

	NETDATA.getDomCharts = function(callback) {
		NETDATA.options.updated_dom = 0;

		NETDATA.options.targets = $('div[data-netdata]').filter(':visible');

		if(NETDATA.options.debug.main_loop)
			console.log('DOM updated - there are ' + NETDATA.options.targets.length + ' charts on page.');

		// we need to re-size all the charts quickly
		// before making any external calls
		$.each(NETDATA.options.targets, function(i, target) {
			// the initialization will take care of sizing
			// and the "loading..." message
			var state = NETDATA.chartState(target);
		});

		if(typeof callback == 'function') callback();
	}

	// this is the main function - where everything starts
	NETDATA.init = function() {
		// this should be called only once

		NETDATA.options.page_is_visible = 1;

		$(window).blur(function() {
			NETDATA.options.page_is_visible = 0;
			if(NETDATA.options.debug.focus) console.log('Lost Focus!');
		});

		$(window).focus(function() {
			NETDATA.options.page_is_visible = 1;
			if(NETDATA.options.debug.focus) console.log('Focus restored!');
		});

		if(typeof document.hasFocus == 'function' && !document.hasFocus()) {
			NETDATA.options.page_is_visible = 0;
			if(NETDATA.options.debug.focus) console.log('Document has no focus!');
		}

		NETDATA.getDomCharts(function() {
			NETDATA.chartRefresher(0);
		});
	}

	// ----------------------------------------------------------------------------------------------------------------

	//var chart = function() {
	//}

	//chart.prototype.color = function() {
	//	return 'red';
	//}

	//var c = new chart();
	//c.color();

	// ----------------------------------------------------------------------------------------------------------------
	// peity

	NETDATA.peityInitialize = function(callback) {
		if(typeof netdataStopPeity == 'undefined') {
			$.getScript(NETDATA.peity_js)
				.done(function() {
					NETDATA.registerChartLibrary('peity', NETDATA.peity_js);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.peity_js);
				})
				.always(function() {
					if(typeof callback == "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.peity.enabled = false;
			if(typeof callback == "function")
				callback();
		}
	};

	NETDATA.peityChartUpdate = function(element, data) {
		var peity = $(data.state.peity_element);
		peity.html(data.result);
		// peity.change() does not accept options
		// to pass width and height
		//peity.change();
		peity.peity('line', { width: data.state.calculated_width, height: data.state.calculated_height });
	}

	NETDATA.peityChartCreate = function(element, data) {
		element.innerHTML = '<div id="peity-' + data.state.uuid + '">' + data.result + '</div>';
		data.state.peity_element = document.getElementById('peity-' + data.state.uuid);
		var peity = $(data.state.peity_element);

		peity.peity('line', { width: data.state.calculated_width, height: data.state.calculated_height });
	}

	// ----------------------------------------------------------------------------------------------------------------
	// sparkline

	NETDATA.sparklineInitialize = function(callback) {
		if(typeof netdataStopSparkline == 'undefined') {
			$.getScript(NETDATA.sparkline_js)
				.done(function() {
					NETDATA.registerChartLibrary('sparkline', NETDATA.sparkline_js);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.sparkline_js);
				})
				.always(function() {
					if(typeof callback == "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.sparkline.enabled = false;
			if(typeof callback == "function") 
				callback();
		}
	};

	NETDATA.sparklineChartUpdate = function(element, data) {
		data.state.sparkline_options.width = data.state.calculated_width;
		data.state.sparkline_options.height = data.state.calculated_height;

		spark = $(data.state.sparkline_element);
		spark.sparkline(data.result, data.state.sparkline_options);
	}

	NETDATA.sparklineChartCreate = function(element, data) {
		var self = $(element);
		var type = self.data('sparkline-type') || 'line';
		var lineColor = self.data('sparkline-linecolor') || NETDATA.colors[0];
		var fillColor = self.data('sparkline-fillcolor') || (data.state.chart.chart_type == 'line')?'#FFF':NETDATA.ColorLuminance(lineColor, 0.8);
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
		var tooltipSuffix = self.data('sparkline-tooltipsuffix') || ' ' + data.state.chart.units;
		var tooltipSkipNull = self.data('sparkline-tooltipskipnull') || true;
		var tooltipValueLookups = self.data('sparkline-tooltipvaluelookups') || undefined;
		var tooltipFormatFieldlist = self.data('sparkline-tooltipformatfieldlist') || undefined;
		var tooltipFormatFieldlistKey = self.data('sparkline-tooltipformatfieldlistkey') || undefined;
		var numberFormatter = self.data('sparkline-numberformatter') || function(n){ return n.toFixed(2); };
		var numberDigitGroupSep = self.data('sparkline-numberdigitgroupsep') || undefined;
		var numberDecimalMark = self.data('sparkline-numberdecimalmark') || undefined;
		var numberDigitGroupCount = self.data('sparkline-numberdigitgroupcount') || undefined;
		var animatedZooms = self.data('sparkline-animatedzooms') || false;

		data.state.sparkline_options = {
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
			tooltipChartTitle: data.state.chart.title,
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
			width: data.state.calculated_width,
			height: data.state.calculated_height
		};

		element.innerHTML = '<div id="sparkline-' + data.state.uuid + '" style="display: inline-block; position: relative;"></div>';
		data.state.sparkline_element = document.getElementById('sparkline-' + data.state.uuid);

		spark = $(data.state.sparkline_element);
		spark.sparkline(data.result, data.state.sparkline_options);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// dygraph

	NETDATA.dygraph = {
		sync: false,
		paused: []
	};

	NETDATA.dygraph.syncStart = function(state, event, x, points, row, seriesName) {
		if(NETDATA.options.debug.dygraph || state.debug) console.log('dygraph.syncStart()');
		state.pauseChart();

		var dygraph = state.dygraph_instance;

		if(!NETDATA.dygraph.sync) {
			$.each(NETDATA.options.targets, function(i, target) {
				var st = NETDATA.chartState(target);
				if(typeof st.dygraph_instance == 'object' && st.library_name == state.library_name && st.canBeAutoRefreshed(false)) {
					NETDATA.dygraph.paused.push(st);
				}
			});
		}

		$.each(NETDATA.dygraph.paused, function(i, st) {
			st.setSelection(x);
		});

	}

	NETDATA.dygraph.syncStop = function(state) {
		if(NETDATA.options.debug.dygraph || state.debug) console.log('dygraph.syncStop()');

		if(!NETDATA.dygraph.sync) {
			$.each(NETDATA.dygraph.paused, function(i, st) {
				st.clearSelection();
			});

			NETDATA.dygraph.sync = false;
			NETDATA.dygraph.paused = [];
		}

		state.unpauseChart();
	}

	NETDATA.dygraph.resetChart = function(element, dygraph) {
		if(NETDATA.options.debug.dygraph) console.log('dygraph.resetChart()');

		state = NETDATA.chartState(element);
		state.resetChart();
		if(NETDATA.globalPanAndZoom.clearMaster());
	}

	NETDATA.dygraph.zoomOrPan = function(element, dygraph, after, before) {
		if(NETDATA.options.debug.dygraph) console.log('>>>> dygraph.zoomOrPan(element, dygraph, after:' + after + ', before: ' + before + ')');

		state = NETDATA.chartState(element);
		state.updateChartPanOrZoom(after, before);
		return;
	}

	NETDATA.dygraphSetSelection = function(state, t) {
		var r = state.calculateRowForTime(t);
		if(r != -1) {
			state.dygraph_instance.setSelection(r);
			state.pauseChart();
		}
		else {
			state.dygraph_instance.clearSelection();
			state.unpauseChart();
		}
	}

	NETDATA.dygraphclearSelection = function(state, t) {
		state.dygraph_instance.clearSelection();
		state.unpauseChart();
	}

	NETDATA.dygraphInitialize = function(callback) {
		if(typeof netdataStopDygraph == 'undefined') {
			$.getScript(NETDATA.dygraph_js)
				.done(function() {
					NETDATA.registerChartLibrary('dygraph', NETDATA.dygraph_js);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.dygraph_js);
				})
				.always(function() {
					if(typeof callback == "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.dygraph.enabled = false;
			if(typeof callback == "function")
				callback();
		}
	};

	NETDATA.dygraphChartUpdate = function(element, data) {
		if(NETDATA.options.debug.dygraph || data.state.debug) console.log('dygraphChartUpdate()');

		var dygraph = data.state.dygraph_instance;

		if(data.state.mode.name == 'pan') {
			if(NETDATA.options.debug.dygraph || data.state.debug) console.log('dygraphChartUpdate() loose update');
			dygraph.updateOptions({
				file: data.result.data,
				labels: data.result.labels,
				labelsDivWidth: data.state.calculated_width - 70
			});
		}
		else {
			if(NETDATA.options.debug.dygraph || data.state.debug) console.log('dygraphChartUpdate() strict update');
			dygraph.updateOptions({
				file: data.result.data,
				labels: data.result.labels,
				labelsDivWidth: data.state.calculated_width - 70,
				dateWindow: null,
    			valueRange: null
			});
		}
	};

	NETDATA.dygraphChartCreate = function(element, data) {
		if(NETDATA.options.debug.dygraph || data.state.debug) console.log('dygraphChartCreate()');

		var self = $(element);
		var title = self.data('dygraph-title') || data.state.chart.title;
		var titleHeight = self.data('dygraph-titleheight') || 20;
		var labelsDiv = self.data('dygraph-labelsdiv') || undefined;
		var connectSeparatedPoints = self.data('dygraph-connectseparatedpoints') || false;
		var yLabelWidth = self.data('dygraph-ylabelwidth') || 12;
		var stackedGraph = self.data('dygraph-stackedgraph') || (data.state.chart.chart_type == 'stacked')?true:false;
		var stackedGraphNaNFill = self.data('dygraph-stackedgraphnanfill') || 'none';
		var hideOverlayOnMouseOut = self.data('dygraph-hideoverlayonmouseout') || true;
		var fillGraph = self.data('dygraph-fillgraph') || (data.state.chart.chart_type == 'area')?true:false;
		var drawPoints = self.data('dygraph-drawpoints') || false;
		var labelsDivStyles = self.data('dygraph-labelsdivstyles') || { 'fontSize':'10px' };
		var labelsDivWidth = self.data('dygraph-labelsdivwidth') || self.width() - 70;
		var labelsSeparateLines = self.data('dygraph-labelsseparatelines') || false;
		var labelsShowZeroValues = self.data('dygraph-labelsshowzerovalues') || true;
		var legend = self.data('dygraph-legend') || 'onmouseover';
		var showLabelsOnHighlight = self.data('dygraph-showlabelsonhighlight') || true;
		var gridLineColor = self.data('dygraph-gridlinecolor') || '#EEE';
		var axisLineColor = self.data('dygraph-axislinecolor') || '#EEE';
		var maxNumberWidth = self.data('dygraph-maxnumberwidth') || 8;
		var sigFigs = self.data('dygraph-sigfigs') || null;
		var digitsAfterDecimal = self.data('dygraph-digitsafterdecimal') || 2;
		var axisLabelFontSize = self.data('dygraph-axislabelfontsize') || 10;
		var axisLineWidth = self.data('dygraph-axislinewidth') || 0.3;
		var drawAxis = self.data('dygraph-drawaxis') || true;
		var strokeWidth = self.data('dygraph-strokewidth') || 1.0;
		var drawGapEdgePoints = self.data('dygraph-drawgapedgepoints') || true;
		var colors = self.data('dygraph-colors') || NETDATA.colors;
		var pointSize = self.data('dygraph-pointsize') || 1;
		var stepPlot = self.data('dygraph-stepplot') || false;
		var strokeBorderColor = self.data('dygraph-strokebordercolor') || 'white';
		var strokeBorderWidth = self.data('dygraph-strokeborderwidth') || (data.state.chart.chart_type == 'stacked')?1.0:0.0;
		var strokePattern = self.data('dygraph-strokepattern') || undefined;
		var highlightCircleSize = self.data('dygraph-highlightcirclesize') || 3;
		var highlightSeriesOpts = self.data('dygraph-highlightseriesopts') || { strokeWidth: 1.5 };
		var highlightSeriesBackgroundAlpha = self.data('dygraph-highlightseriesbackgroundalpha') || (data.state.chart.chart_type == 'stacked')?0.7:0.5;
		var pointClickCallback = self.data('dygraph-pointclickcallback') || undefined;
		var showRangeSelector = self.data('dygraph-showrangeselector') || false;
		var showRoller = self.data('dygraph-showroller') || false;
		var valueFormatter = self.data('dygraph-valueformatter') || undefined; //function(x){ return x.toFixed(2); };
		var rightGap = self.data('dygraph-rightgap') || 5;
		var drawGrid = self.data('dygraph-drawgrid') || true;
		var drawXGrid = self.data('dygraph-drawxgrid') || undefined;
		var drawYGrid = self.data('dygraph-drawygrid') || undefined;
		var gridLinePattern = self.data('dygraph-gridlinepattern') || null;
		var gridLineWidth = self.data('dygraph-gridlinewidth') || 0.3;

		data.state.dygraph_options = {
			title: title,
			titleHeight: titleHeight,
			ylabel: data.state.chart.units,
			yLabelWidth: yLabelWidth,
			connectSeparatedPoints: connectSeparatedPoints,
			drawPoints: drawPoints,
			fillGraph: fillGraph,
			stackedGraph: stackedGraph,
			stackedGraphNaNFill: stackedGraphNaNFill,
			drawGrid: drawGrid,
			drawXGrid: drawXGrid,
			drawYGrid: drawYGrid,
			gridLinePattern: gridLinePattern,
			gridLineWidth: gridLineWidth,
			gridLineColor: gridLineColor,
			axisLineColor: axisLineColor,
			axisLineWidth: axisLineWidth,
			drawAxis: drawAxis,
			hideOverlayOnMouseOut: hideOverlayOnMouseOut,
			labelsDiv: labelsDiv,
			labelsDivStyles: labelsDivStyles,
			labelsDivWidth: labelsDivWidth,
			labelsSeparateLines: labelsSeparateLines,
			labelsShowZeroValues: labelsShowZeroValues,
			labelsKMB: false,
			labelsKMG2: false,
			legend: legend,
			showLabelsOnHighlight: showLabelsOnHighlight,
			maxNumberWidth: maxNumberWidth,
			sigFigs: sigFigs,
			digitsAfterDecimal: digitsAfterDecimal,
			axisLabelFontSize: axisLabelFontSize,
			colors: colors,
			strokeWidth: strokeWidth,
			drawGapEdgePoints: drawGapEdgePoints,
			pointSize: pointSize,
			stepPlot: stepPlot,
			strokeBorderColor: strokeBorderColor,
			strokeBorderWidth: strokeBorderWidth,
			strokePattern: strokePattern,
			highlightCircleSize: highlightCircleSize,
			highlightSeriesOpts: highlightSeriesOpts,
			highlightSeriesBackgroundAlpha: highlightSeriesBackgroundAlpha,
			pointClickCallback: pointClickCallback,
			showRangeSelector: showRangeSelector,
			showRoller: showRoller,
			valueFormatter: valueFormatter,
			rightGap: rightGap,
			labels: data.result.labels,
			axes: {
				x: {
					pixelsPerLabel: 50,
					ticker: Dygraph.dateTicker,
					axisLabelFormatter: function (d, gran) {
						return Dygraph.zeropad(d.getHours()) + ":" + Dygraph.zeropad(d.getMinutes()) + ":" + Dygraph.zeropad(d.getSeconds());
					},
					valueFormatter :function (ms) {
						var d = new Date(ms);
						return Dygraph.zeropad(d.getHours()) + ":" + Dygraph.zeropad(d.getMinutes()) + ":" + Dygraph.zeropad(d.getSeconds());
					}
				},
				y: {
					pixelsPerLabel: 15
				}
			},
			drawCallback: function(dygraph, is_initial) {
				if(data.state.mode.name != 'auto') {
					if(NETDATA.options.debug.dygraph) console.log('dygraphDrawCallback()');

					var x_range = dygraph.xAxisRange();
					var after = Math.round(x_range[0]);
					var before = Math.round(x_range[1]);

					NETDATA.dygraph.zoomOrPan(element, this, after, before);
				}
			},
			zoomCallback: function(minDate, maxDate, yRanges) {
				if(NETDATA.options.debug.dygraph) console.log('dygraphZoomCallback()');
				NETDATA.dygraph.zoomOrPan(element, this, minDate, maxDate);
			},
			highlightCallback: function(event, x, points, row, seriesName) {
				if(NETDATA.options.debug.dygraph) console.log('dygraphHighlightCallback()');
				NETDATA.dygraph.syncStart(data.state, event, x, points, row, seriesName);
			},
			unhighlightCallback: function(event) {
				if(NETDATA.options.debug.dygraph) console.log('dygraphUnhighlightCallback()');
				NETDATA.dygraph.syncStop(data.state);
			},
			interactionModel : {
				mousedown: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph) console.log('dygraphMouseDown()');

					// Right-click should not initiate a zoom.
					if (event.button && event.button == 2) return;

					context.initializeMouseDown(event, dygraph, context);
					
					if (event.altKey || event.shiftKey) {
						data.state.setMode('zoom');
						Dygraph.startZoom(event, dygraph, context);
					}
					else {
						data.state.setMode('pan');
						Dygraph.startPan(event, dygraph, context);
					}
				},
				mousemove: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph) console.log('dygraphMouseMove()');

					if (context.isPanning) {
						data.state.setMode('pan');
						Dygraph.movePan(event, dygraph, context);
					}
					else if (context.isZooming) {
						data.state.setMode('zoom');
						Dygraph.moveZoom(event, dygraph, context);
					}
				},
				mouseup: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph) console.log('dygraphMouseUp()');

					if (context.isPanning)
						Dygraph.endPan(event, dygraph, context);
					else if (context.isZooming)
						Dygraph.endZoom(event, dygraph, context);
				},
				click: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph) console.log('dygraphMouseClick()');
					Dygraph.cancelEvent(event);
				},
				dblclick: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph) console.log('dygraphMouseDoubleClick()');
					NETDATA.dygraph.resetChart(element, dygraph);
				},
				mousewheel: function(event, dygraph, context) {
					if(NETDATA.options.debug.dygraph) console.log('dygraphMouseWheel()');

					if(event.altKey || event.shiftKey) {
						// http://dygraphs.com/gallery/interaction-api.js
						var normal = (event.detail) ? event.detail * -1 : event.wheelDelta / 40;
						var percentage = normal / 25;

						var before_old = data.state.mode.before_ms;
						var after_old = data.state.mode.after_ms;
						var range_old = before_old - after_old;

						var range = range_old * ( 1 - percentage );
						var dt = Math.round((range_old - range) / 2);

						var before = before_old - dt;
						var after  = after_old  + dt;

						if(NETDATA.options.debug.dygraph) console.log('percent: ' + percentage + ' from ' + after_old + ' - ' + before_old + ' to ' + after + ' - ' + before + ', range from ' + (before_old - after_old).toString() + ' to ' + (before - after).toString());

						data.state.setMode('zoom');
						NETDATA.dygraph.zoomOrPan(element, dygraph, after, before);
					}					
				},
				touchstart: function(event, dygraph, context) {
					Dygraph.Interaction.startTouch(event, dygraph, context);
					context.touchDirections = { x: true, y: false };
					data.state.setMode('zoom');
				},
				touchmove: function(event, dygraph, context) {
					//Dygraph.cancelEvent(event);
					Dygraph.Interaction.moveTouch(event, dygraph, context);
				},
				touchend: function(event, dygraph, context) {
					Dygraph.Interaction.endTouch(event, dygraph, context);
				}
			}
		};

		self.html('<div id="dygraph-' + data.state.uuid + '" style="width: 100%; height: 100%;"></div>');

		data.state.dygraph_instance = new Dygraph(document.getElementById('dygraph-' + data.state.uuid),
			data.result.data, data.state.dygraph_options);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// morris

	NETDATA.morrisInitialize = function(callback) {
		if(typeof netdataStopMorris == 'undefined') {

			// morris requires raphael
			if(!NETDATA.chartLibraries.raphael.initialized) {
				if(NETDATA.chartLibraries.raphael.enabled) {
					NETDATA.raphaelInitialize(function() {
						NETDATA.morrisInitialize(callback);
					});
				}
				else {
					NETDATA.chartLibraries.morris.enabled = false;
					if(typeof callback == "function")
						callback();
				}
			}
			else {
				var fileref = document.createElement("link");
				fileref.setAttribute("rel", "stylesheet");
				fileref.setAttribute("type", "text/css");
				fileref.setAttribute("href", NETDATA.morris_css);

				if (typeof fileref != "undefined")
					document.getElementsByTagName("head")[0].appendChild(fileref);

				$.getScript(NETDATA.morris_js)
					.done(function() {
						NETDATA.registerChartLibrary('morris', NETDATA.morris_js);
					})
					.fail(function() {
						NETDATA.error(100, NETDATA.morris_js);
					})
					.always(function() {
						if(typeof callback == "function")
							callback();
					})
			}
		}
		else {
			NETDATA.chartLibraries.morris.enabled = false;
			if(typeof callback == "function")
				callback();
		}
	};

	NETDATA.morrisChartUpdate = function(element, data) {
		data.state.morris_instance.setData(data.result.data);
	};

	NETDATA.morrisChartCreate = function(element, data) {

		element.innerHTML = '<div id="morris-' + data.state.uuid + '" style="width: ' + data.state.calculated_width + 'px; height: ' + data.state.calculated_height + 'px;"></div>';

		var options = {
				element: 'morris-' + data.state.uuid,
				data: data.result.data,
				xkey: 'time',
				ykeys: data.dimension_names,
				labels: data.dimension_names,
				lineWidth: 2,
				pointSize: 2,
				smooth: true,
				hideHover: 'auto',
				parseTime: true,
				continuousLine: false,
				behaveLikeLine: false
		};

		var morris;
		if(data.state.chart.chart_type == 'line')
			morris = new Morris.Line(options);

		else if(data.state.chart.chart_type == 'area') {
			options.behaveLikeLine = true;
			morris = new Morris.Area(options);
		}
		else // stacked
			morris = new Morris.Area(options);

		data.state.morris_instance = morris;
		data.state.morris_options = options;
	};

	// ----------------------------------------------------------------------------------------------------------------
	// raphael

	NETDATA.raphaelInitialize = function(callback) {
		if(typeof netdataStopRaphael == 'undefined') {
			$.getScript(NETDATA.raphael_js)
				.done(function() {
					NETDATA.registerChartLibrary('raphael', NETDATA.raphael_js);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.raphael_js);
				})
				.always(function() {
					if(typeof callback == "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.raphael.enabled = false;
			if(typeof callback == "function")
				callback();
		}
	};

	NETDATA.raphaelChartUpdate = function(element, data) {
		var self = $(element);

		self.raphael(data, {
			width: data.state.calculated_width,
			height: data.state.calculated_height
		})
	};

	NETDATA.raphaelChartCreate = function(element, data) {
		var self = $(element);

		self.raphael(data, {
			width: data.state.calculated_width,
			height: data.state.calculated_height
		})
	};

	// ----------------------------------------------------------------------------------------------------------------
	// google charts

	NETDATA.googleInitialize = function(callback) {
		if(typeof netdataStopGoogleCharts == 'undefined') {
			$.getScript(NETDATA.google_js)
				.done(function() {
					NETDATA.registerChartLibrary('google', NETDATA.google_js);

					google.load('visualization', '1.1', {
						'packages': ['corechart', 'controls'],
						'callback': callback
					});
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.google_js);
					if(typeof callback == "function")
						callback();
				})
		}
		else {
			NETDATA.chartLibraries.google.enabled = false;
			if(typeof callback == "function")
				callback();
		}
	};

	NETDATA.googleChartUpdate = function(element, data) {
		var datatable = new google.visualization.DataTable(data.result);
		data.state.google_instance.draw(datatable, data.state.google_options);
	};

	NETDATA.googleChartCreate = function(element, data) {
		var datatable = new google.visualization.DataTable(data.result);

		var options = {
			// do not set width, height - the chart resizes itself
			//width: data.state.calculated_width,
			//height: data.state.calculated_height,
			lineWidth: 1,
			title: data.state.chart.title,
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
				title: data.state.chart.units,
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

		element.innerHTML = '<div id="google-' + data.state.uuid + '" style="width: 100%; height: 100%;"></div>';

		var gchart;
		switch(data.state.chart.chart_type) {
			case "area":
				options.vAxis.viewWindowMode = 'maximized';
				gchart = new google.visualization.AreaChart(document.getElementById('google-' + data.state.uuid));
				break;

			case "stacked":
				options.isStacked = true;
				options.areaOpacity = 0.85;
				options.vAxis.viewWindowMode = 'maximized';
				options.vAxis.minValue = null;
				options.vAxis.maxValue = null;
				gchart = new google.visualization.AreaChart(document.getElementById('google-' + data.state.uuid));
				break;

			default:
			case "line":
				options.lineWidth = 2;
				gchart = new google.visualization.LineChart(document.getElementById('google-' + data.state.uuid));
				break;
		}

		gchart.draw(datatable, options);

		data.state.google_instance = gchart;
		data.state.google_options = options;
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
			format: 'json',
			options: 'ms|flip',
			jsonWrapper: true,
			pixels_per_point: 2,
			detects_dimensions_on_update: false
		},
		"sparkline": {
			initialize: NETDATA.sparklineInitialize,
			create: NETDATA.sparklineChartCreate,
			update: NETDATA.sparklineChartUpdate,
			setSelection: null,
			clearSelection: null,
			initialized: false,
			enabled: true,
			format: 'array',
			options: 'flip|abs',
			jsonWrapper: true,
			pixels_per_point: 2,
			detects_dimensions_on_update: false
		},
		"peity": {
			initialize: NETDATA.peityInitialize,
			create: NETDATA.peityChartCreate,
			update: NETDATA.peityChartUpdate,
			setSelection: null,
			clearSelection: null,
			initialized: false,
			enabled: true,
			format: 'ssvcomma',
			options: 'null2zero|flip|abs',
			jsonWrapper: true,
			pixels_per_point: 2,
			detects_dimensions_on_update: false
		},
		"morris": {
			initialize: NETDATA.morrisInitialize,
			create: NETDATA.morrisChartCreate,
			update: NETDATA.morrisChartUpdate,
			setSelection: null,
			clearSelection: null,
			initialized: false,
			enabled: true,
			format: 'json',
			options: 'objectrows|ms',
			jsonWrapper: true,
			pixels_per_point: 10,
			detects_dimensions_on_update: false
		},
		"google": {
			initialize: NETDATA.googleInitialize,
			create: NETDATA.googleChartCreate,
			update: NETDATA.googleChartUpdate,
			setSelection: null,
			clearSelection: null,
			initialized: false,
			enabled: true,
			format: 'datatable',
			options: '',
			jsonWrapper: true,
			pixels_per_point: 2,
			detects_dimensions_on_update: true
		},
		"raphael": {
			initialize: NETDATA.raphaelInitialize,
			create: NETDATA.raphaelChartCreate,
			update: NETDATA.raphaelChartUpdate,
			setSelection: null,
			clearSelection: null,
			initialized: false,
			enabled: true,
			format: 'json',
			options: '',
			jsonWrapper: true,
			pixels_per_point: 1,
			detects_dimensions_on_update: false
		}
	};

	NETDATA.registerChartLibrary = function(library, url) {
		console.log("registering chart library: " + library);

		NETDATA.chartLibraries[library].url = url;
		NETDATA.chartLibraries[library].initialized = true;
		NETDATA.chartLibraries[library].enabled = true;

		// console.log(NETDATA.chartLibraries);
	}

	// ----------------------------------------------------------------------------------------------------------------
	// load all libraries and initialize

	NETDATA.errorReset();

	NETDATA._loadjQuery(function() {
		$.getScript(NETDATA.serverDefault + 'lib/visible.js').then(function() {
			NETDATA.init();
		})
	});

})(window);
