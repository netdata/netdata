// You can set the following variables before loading this script:
//
// var netdataStopDygraph = 1;			// do not use dygraph
// var netdataStopSparkline = 1;		// do not use sparkline
// var netdataStopPeity = 1;			// do not use peity
//
// You can also set the default netdata server, using the following.
// When this variable is not set, we assume the page is hosted on your
// netdata server already.
// var netdataServer = "http://yourhost:19999"; // set your NetData server

// --------------------------------------------------------------------------------------------------------------------
// For google charts you need this in your page:
//	<script type="text/javascript" src="https://www.google.com/jsapi"></script>
//	<script type='text/javascript'>google.load('visualization', '1.1', {'packages':['corechart', 'controls']});</script>

(function(window)
{
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
	NETDATA.colors	= [ '#3366CC', '#DC3912', '#FF9900', '#109618', '#990099', '#3B3EAC', '#0099C6', '#DD4477',
		'#66AA00', '#B82E2E', '#316395', '#994499', '#22AA99', '#AAAA11', '#6633CC', '#E67300', '#8B0707',
		'#329262', '#5574A6', '#3B3EAC' ];

	// ----------------------------------------------------------------------------------------------------------------
	// the defaults for all charts

	NETDATA.chartDefaults = {
		host: NETDATA.serverDefault,	// the server to get data from
		width: 100,						// the chart width
		height: 20,						// the chart height
		library: 'peity',				// the graphing library to use
		method: 'average',				// the grouping method
		before: 0,						// panning
		after: -600,					// panning
		point_width: 1,					// the detail of the chart
	}

	NETDATA.all_url = 'all2.js';
	NETDATA_idle_between_charts = 50;
	NETDATA.debug = 1;

	if(NETDATA.debug) console.log('welcome to NETDATA');


	// ----------------------------------------------------------------------------------------------------------------
	// Error Handling

	NETDATA.errorCodes = {
		100: { message: "Cannot load chart library", alert: true },
		101: { message: "Cannot load jQuery", alert: true },
		402: { message: "Chart library not found", alert: false },
		404: { message: "Chart not found", alert: false },
	};
	NETDATA.errorLast = {
		code: 0,
		message: "",
		datetime: 0,
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
	}

	NETDATA.messageInABox = function(div, width, height, message) {
		div.innerHTML = '<table border="0"><tr><td width="' + width + '" height="' + height
			+ '" valign="middle" align="center">'
			+ message
			+ '</td></tr></table>';
	}

	// ----------------------------------------------------------------------------------------------------------------
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

	NETDATA.generateDataURL = function(args) {
		// build the data URL
		var url = args.host + args.url;
		url += "&points=";
		url += args.points.toString();
		url += "&group=";
		url += args.method;
		url += "&after=";
		url += args.after || "0";
		url += "&before=";
		url += args.before || "0";
		url += "&options=" + NETDATA.chartLibraries[args.library].options + '|';
		url += (args.non_zero)?"nonzero":"";
		url += "&format=" + NETDATA.chartLibraries[args.library].format;

		if(args.dimensions)
			url += "&dimensions=" + args.dimensions;

		if(NETDATA.debug) console.log('generateDataURL(' + args + ') = ' + url );
		return url;
	}

	NETDATA.loadCharts = function(targets, index, callback) {
		if(NETDATA.debug) console.log('loadCharts(<targets, ' + index + ')');

		var target = targets.get(index);

		if(target == null) {
			console.log('loaded all charts');
			callback();
		}
		else {
			var self = $(target);

			if(!self.data('loaded')) {
				var id = self.data('netdata');

				var host = self.data('host') || NETDATA.chartDefaults.host;
				var width = self.data('width') || NETDATA.chartDefaults.width;
				var height = self.data('height') || NETDATA.chartDefaults.height;
				var method = self.data('method') || NETDATA.chartDefaults.method;
				var after = self.data('after') || NETDATA.chartDefaults.after;
				var before = self.data('before') || NETDATA.chartDefaults.before;
				var host = self.data('host') || NETDATA.chartDefaults.host;
				var library = self.data('chart-library') || NETDATA.chartDefaults.library;
				var dimensions = self.data('dimensions') || null;

				if(typeof NETDATA.chartLibraries[library] == 'undefined') {
					NETDATA.error(402, library);
					NETDATA.messageInABox(target, width, height, 'chart library "' + library + '" is not found');
					self.data('enabled', false);
					NETDATA.loadCharts(targets, ++index, callback);
				}
				else {
					var url = host + "/api/v1/chart?chart=" + id; 

					$.ajax( {
						url:  url,
						crossDomain: true
					})
					.done(function(chart) {

						var point_width = self.data('point-width') || NETDATA.chartLibraries[library].pixels;
						var points = self.data('points') || Math.round(width / point_width);

						var url = NETDATA.generateDataURL({
							host: host,
							url: chart.data_url,
							dimensions: dimensions,
							library: library,
							method: method,
							before: before,
							points: points,
							after: after,
							non_zero: null
						});

						// done processing of this DIV
						// store the processing result, in
						// 'data' sections in the DIV
						self
							.data('chart', chart)
							.data('chart-url', url)
							.data('last-updated', 0)
							.data('update-every', chart.update_every * 1000)
							.data('enabled', true);
					})
					.fail(function() {
						NETDATA.error(404, url);
						NETDATA.messageInABox(target, width, height, 'chart "' + id + '" not found on url "' + url + '"');
						self.data('enabled', false);
					})
					.always(function() {
						NETDATA.loadCharts(targets, ++index, callback);
					});
				}
			}
		}
	}

	function netdataParsePageCharts() {
		if(NETDATA.debug) console.log('processing web page defined charts');

		// targets
		// a list of all DIVs containing netdata charts

		var targets = $('div[data-netdata]')
		.bind('create', function(event, data) {
			var self = $(this);
			var lib = self.data('chart-library') || 'dygraph';
			var method = lib + 'ChartCreate';
			
			console.log('Calling ' + method + '()');
			NETDATA[method].apply(this, arguments);
		})
		.bind('update', function() {
			var self = $(this);
			var lib = self.data('chart-library') || 'dygraph';
			var method = lib + 'ChartUpdate';
			
			console.log('Calling ' + method + '()');
			NETDATA[method].apply(this, arguments);
		});

		NETDATA.loadCharts(targets, 0, function() {
			// done processing all netdata DIVs in this page
			// call the refresh handler
			netdataRefreshTargets(targets, 0);
		});
	}

	// ----------------------------------------------------------------------------------------------------------------
	// Charts Libraries Registration

	NETDATA.chartLibraries = {};

	NETDATA.registerChartLibrary = function(library, url, format, options, min_pixels_per_point) {
		console.log("registering chart library: " + library);

		NETDATA.chartLibraries[library] = {
			initialized: new Date().getTime(),
			url: url,
			format: format,
			options: options,
			pixels: min_pixels_per_point
		};

		console.log(NETDATA.chartLibraries);
	}

	// ----------------------------------------------------------------------------------------------------------------

	//var chart = function() {
	//}

	//chart.prototype.color = function() {
	//	return 'red';
	//}

	//var c = new chart();
	//c.color();

	function netdataDownloadChartData(callback) {
		var self = $(this);
		var last = self.data('last-updated') || 0;
		var every = self.data('update-every') || 1;

		// check if this chart has to be refreshed now
		var now = new Date().getTime();
		if(last + every > now) {
			if(typeof callback == 'function')
				callback();
		}
		else {
			var url = self.data('chart-url');

			if(NETDATA.debug) console.log('netdataDownloadChartData(): downloading ' + url);

			$.ajax( {
				url: url,
				crossDomain: true
			})
			.then(function(data) {
				//var result = $.map(data.rows, function(item) {
					// get from the 3rd column the 'v' member
					//return item.c[3].v;
				//});
				
				// since we downloaded the data
				// update the last-updated time to prevent
				// another download too soon
				self.data('last-updated', new Date().getTime());

				// render it
				var created = self.data('created');
				self.trigger((created ? 'update' : 'create'), [data]).data('created', true)
			})
			.fail(function() {
				console.log('failed to download chart data');
			})
			.always(function() {
				var last = self.data('last-updated');
				var now = new Date().getTime();
				var dt = now - last;
				self.data('refresh-dt', dt);

				var element_name = self.data('dt-element-name') || null;
				if(element_name) {
					var element = document.getElementById(element_name) || null;
					if(element) {
						element.innerHTML = dt.toString();
					}
				}

				if(typeof callback == 'function')
					callback();
			});
		}
	};

	function netdataRefreshTargets(targets, index) {
		// if(NETDATA.debug) console.log('netdataRefreshTargets(<targets, ' + index + ')');

		var target = targets.get(index);

		if(target == null) {
			console.log('restart');

			// finished, restart
			setTimeout(function() {
				netdataRefreshTargets(targets, 0);
			}, 1000);
		}
		else {
			var self = $(target);

			if(!self.data('enabled')) {
				netdataRefreshTargets(targets, ++index);
			}
			else {
				setTimeout(function() {
					netdataDownloadChartData.call(target, function() {
						netdataRefreshTargets(targets, ++index);
					});
				}, NETDATA_idle_between_charts);
			}
		}
	}


	// ----------------------------------------------------------------------------------------------------------------
	// piety

	NETDATA.peityInitialize = function(callback) {
		if(typeof netdataStopPeity == 'undefined') {
			$.getScript(NETDATA.peity_js)
				.done(function() {
					NETDATA.registerChartLibrary('peity', NETDATA.peity_js, 'ssvcomma', 'null2zero|flip|min2max', 2);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.peity_js);
				})
				.always(function() {
					if(typeof callback == "function")
						callback();
				})
		}
		else if(typeof callback == "function")
			callback();
	};

	NETDATA.peityChartUpdate = function(event, data) {
		var self = $(this);
		var instance = self.html(data).not('[data-created]');
		instance.change();
	}

	NETDATA.peityChartCreate = function(event, data) {
		var self = $(this);
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		var instance = self.html(data).not('[data-created]');

		instance.peity('line', {
			width: width,
			height: height
		})
		.data('created', true);
	}

	// ----------------------------------------------------------------------------------------------------------------
	// sparkline

	NETDATA.sparklineInitialize = function(callback) {
		if(typeof netdataStopSparkline == 'undefined') {
			$.getScript(NETDATA.sparkline_js)
				.done(function() {
					NETDATA.registerChartLibrary('sparkline', NETDATA.sparkline_js, 'array', 'flip|min2max', 2);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.sparkline_js);
				})
				.always(function() {
					if(typeof callback == "function")
						callback();
				})
		}
		else if(typeof callback == "function")
			callback();
	};

	NETDATA.sparklineChartUpdate = function(event, data) {
		var self = $(this);
		var options = self.data('sparkline-options');
		self.sparkline(data, options);
	}

	NETDATA.sparklineChartCreate = function(event, data) {
		var self = $(this);
		var chart = self.data('chart');
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		var type = self.data('sparkline-type') || 'line';
		var lineColor = self.data('sparkline-lineColor') || undefined;
		var fillColor = self.data('sparkline-fillColor') || (chart.chart_type == 'line')?'#FFF':undefined;
		var chartRangeMin = self.data('sparkline-chartRangeMin') || undefined;
		var chartRangeMax = self.data('sparkline-chartRangeMax') || undefined;
		var composite = self.data('sparkline-composite') || undefined;
		var enableTagOptions = self.data('sparkline-enableTagOptions') || undefined;
		var tagOptionPrefix = self.data('sparkline-tagOptionPrefix') || undefined;
		var tagValuesAttribute = self.data('sparkline-tagValuesAttribute') || undefined;
		var disableHiddenCheck = self.data('sparkline-disableHiddenCheck') || undefined;
		var defaultPixelsPerValue = self.data('sparkline-defaultPixelsPerValue') || undefined;
		var spotColor = self.data('sparkline-spotColor') || undefined;
		var minSpotColor = self.data('sparkline-minSpotColor') || undefined;
		var maxSpotColor = self.data('sparkline-maxSpotColor') || undefined;
		var spotRadius = self.data('sparkline-spotRadius') || undefined;
		var valueSpots = self.data('sparkline-valueSpots') || undefined;
		var highlightSpotColor = self.data('sparkline-highlightSpotColor') || undefined;
		var highlightLineColor = self.data('sparkline-highlightLineColor') || undefined;
		var lineWidth = self.data('sparkline-lineWidth') || undefined;
		var normalRangeMin = self.data('sparkline-normalRangeMin') || undefined;
		var normalRangeMax = self.data('sparkline-normalRangeMax') || undefined;
		var drawNormalOnTop = self.data('sparkline-drawNormalOnTop') || undefined;
		var xvalues = self.data('sparkline-xvalues') || undefined;
		var chartRangeClip = self.data('sparkline-chartRangeClip') || undefined;
		var xvalues = self.data('sparkline-xvalues') || undefined;
		var chartRangeMinX = self.data('sparkline-chartRangeMinX') || undefined;
		var chartRangeMaxX = self.data('sparkline-chartRangeMaxX') || undefined;
		var disableInteraction = self.data('sparkline-disableInteraction') || false;
		var disableTooltips = self.data('sparkline-disableTooltips') || false;
		var disableHighlight = self.data('sparkline-disableHighlight') || false;
		var highlightLighten = self.data('sparkline-highlightLighten') || 1.4;
		var highlightColor = self.data('sparkline-highlightColor') || undefined;
		var tooltipContainer = self.data('sparkline-tooltipContainer') || undefined;
		var tooltipClassname = self.data('sparkline-tooltipClassname') || undefined;
		var tooltipFormat = self.data('sparkline-tooltipFormat') || undefined;
		var tooltipPrefix = self.data('sparkline-tooltipPrefix') || undefined;
		var tooltipSuffix = self.data('sparkline-tooltipSuffix') || ' ' + chart.units;
		var tooltipSkipNull = self.data('sparkline-tooltipSkipNull') || true;
		var tooltipValueLookups = self.data('sparkline-tooltipValueLookups') || undefined;
		var tooltipFormatFieldlist = self.data('sparkline-tooltipFormatFieldlist') || undefined;
		var tooltipFormatFieldlistKey = self.data('sparkline-tooltipFormatFieldlistKey') || undefined;
		var numberFormatter = self.data('sparkline-numberFormatter') || function(n){ return n.toFixed(2); };
		var numberDigitGroupSep = self.data('sparkline-numberDigitGroupSep') || undefined;
		var numberDecimalMark = self.data('sparkline-numberDecimalMark') || undefined;
		var numberDigitGroupCount = self.data('sparkline-numberDigitGroupCount') || undefined;
		var animatedZooms = self.data('sparkline-animatedZooms') || false;
 
		var options = {
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
			tooltipChartTitle: chart.title,
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
			width: width,
			height: height
		};

		self.sparkline(data, options);
		self.data('sparkline-options', options)
		.data('created', true);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// dygraph

	NETDATA.dygraphAllCharts = [];
	NETDATA.dygraphInitSync = function(callback) {
		//$.getScript(NETDATA.serverDefault + 'dygraph-synchronizer.js')
		//	.always(function() {
				if(typeof callback == "function")
					callback();
		//	})
	}

	NETDATA.dygraphSync = null;
	NETDATA.dygraphSyncAll = function() {
		if(NETDATA.dygraphSync) {
			NETDATA.dygraphSync.detach();
			NETDATA.dygraphSync = null;
		}

		NETDATA.dygraphSync = Dygraph.synchronize(NETDATA.dygraphAllCharts, {
 			selection: true,
 			zoom: false
 		});
	}

	NETDATA.dygraphInitialize = function(callback) {
		if(typeof netdataStopDygraph == 'undefined') {
			$.getScript(NETDATA.dygraph_js)
				.done(function() {
					NETDATA.registerChartLibrary('dygraph', NETDATA.dygraph_js, 'json', 'ms|flip', 2);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.dygraph_js);
				})
				.always(function() {
					NETDATA.dygraphInitSync(callback);
				})
		}
		else if(typeof callback == "function")
			callback();
	};

	NETDATA.dygraphChartUpdate = function(event, data) {
		var self = $(this);
		var dygraph = self.data('dygraph-instance');

		if(typeof data.update_every != 'undefined')
			self.data('update-every', data.update_every * 1000);

		if(dygraph != null) {
			console.log('updating dygraphs');
			dygraph.updateOptions( { 'file': data.data, 'labels': data.labels } );
		}
		else
			console.log('not updating dygraphs');
	};
 
	NETDATA.dygraphChartCreate = function(event, data) {
		var self = $(this);
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		var chart = self.data('chart');
		var title = self.data('dygraph-title') || chart.title;
		var titleHeight = self.data('dygraph-titleHeight') || 20;
		var labelsDiv = self.data('dygraph-labelsDiv') || undefined;
		var connectSeparatedPoints = self.data('dygraph-connectSeparatedPoints') || false;
		var yLabelWidth = self.data('dygraph-yLabelWidth') || 12;
		var stackedGraph = self.data('dygraph-stackedGraph') || (chart.chart_type == 'stacked')?true:false;
		var stackedGraphNaNFill = self.data('dygraph-stackedGraphNaNFill') || 'none';
		var hideOverlayOnMouseOut = self.data('dygraph-hideOverlayOnMouseOut') || true;
		var fillGraph = self.data('dygraph-fillGraph') || (chart.chart_type == 'area')?true:false;
		var drawPoints = self.data('dygraph-drawPoints') || false;
		var labelsDivStyles = self.data('dygraph-labelsDivStyles') || { 'fontSize':'10' };
		var labelsDivWidth = self.data('dygraph-labelsDivWidth') || 250;
		var labelsSeparateLines = self.data('dygraph-labelsSeparateLines') || false;
		var labelsShowZeroValues = self.data('dygraph-labelsShowZeroValues') || true;
		var legend = self.data('dygraph-legend') || 'onmouseover';
		var showLabelsOnHighlight = self.data('dygraph-showLabelsOnHighlight') || true;
		var gridLineColor = self.data('dygraph-gridLineColor') || '#EEE';
		var axisLineColor = self.data('dygraph-axisLineColor') || '#EEE';
		var maxNumberWidth = self.data('dygraph-maxNumberWidth') || 8;
		var sigFigs = self.data('dygraph-sigFigs') || null;
		var digitsAfterDecimal = self.data('dygraph-digitsAfterDecimal') || 2;
		var axisLabelFontSize = self.data('dygraph-axisLabelFontSize') || 10;
		var axisLineWidth = self.data('dygraph-axisLineWidth') || 0.3;
		var drawAxis = self.data('dygraph-drawAxis') || true;
		var strokeWidth = self.data('dygraph-strokeWidth') || 1.0;
		var drawGapEdgePoints = self.data('dygraph-drawGapEdgePoints') || true;
		var colors = self.data('dygraph-colors') || NETDATA.colors;
		var pointSize = self.data('dygraph-pointSize') || 1;
		var stepPlot = self.data('dygraph-stepPlot') || false;
		var strokeBorderColor = self.data('dygraph-strokeBorderColor') || 'white';
		var strokeBorderWidth = self.data('dygraph-strokeBorderWidth') || (chart.chart_type == 'stacked')?1.0:0.0;
		var strokePattern = self.data('dygraph-strokePattern') || undefined;
		var highlightCircleSize = self.data('dygraph-highlightCircleSize') || 3;
		var highlightSeriesOpts = self.data('dygraph-highlightSeriesOpts') || { strokeWidth: 1.5 };
		var highlightSeriesBackgroundAlpha = self.data('dygraph-highlightSeriesBackgroundAlpha') || (chart.chart_type == 'stacked')?0.7:0.5;
		var pointClickCallback = self.data('dygraph-pointClickCallback') || undefined;
		var showRangeSelector = self.data('dygraph-showRangeSelector') || false;
		var showRoller = self.data('dygraph-showRoller') || false;
		var valueFormatter = self.data('dygraph-valueFormatter') || undefined; //function(x){ return x.toFixed(2); };
		var rightGap = self.data('dygraph-rightGap') || 5;
		var drawGrid = self.data('dygraph-drawGrid') || true;
		var drawXGrid = self.data('dygraph-drawXGrid') || undefined;
		var drawYGrid = self.data('dygraph-drawYGrid') || undefined;
		var gridLinePattern = self.data('dygraph-gridLinePattern') || null;
		var gridLineWidth = self.data('dygraph-gridLineWidth') || 0.3;
  
		var options = {
			title: title,
			titleHeight: titleHeight,
			ylabel: chart.units,
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
			width: width,
			height: height,
			labels: data.labels,
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
					pixelsPerLabel: 15,
				}
			}
		};

		self.html('<div id="dygraph-' + chart.id + '"></div>');

		var dchart = new Dygraph(document.getElementById('dygraph-' + chart.id),
			data.data, options);

		self.data('dygraph-instance', dchart)
		.data('dygraph-options', options)
		.data('created', true);

		//NETDATA.dygraphAllCharts.push(dchart);
		//if(NETDATA.dygraphAllCharts.length > 1)
		//	NETDATA.dygraphSyncAll();
	};

	// ----------------------------------------------------------------------------------------------------------------
	// morris

	NETDATA.morrisInitialize = function(callback) {
		if(typeof netdataStopMorris == 'undefined') {
			var fileref = document.createElement("link");
			fileref.setAttribute("rel", "stylesheet");
			fileref.setAttribute("type", "text/css");
			fileref.setAttribute("href", NETDATA.morris_css);

			if (typeof fileref != "undefined")
				document.getElementsByTagName("head")[0].appendChild(fileref);

			$.getScript(NETDATA.morris_js)
				.done(function() {
					NETDATA.registerChartLibrary('morris', NETDATA.morris_js, 'json', 'objectrows|ms', 10);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.morris_js);
				})
				.always(function() {
					if(typeof callback == "function")
						callback();
				})
		}
		else if(typeof callback == "function")
			callback();
	};

	NETDATA.morrisChartUpdate = function(event, data) {
		var self = $(this);
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		var morris = self.data('morris-instance');

		if(typeof data.update_every != 'undefined')
			self.data('update-every', data.update_every * 1000);

		if(morris != null) {
			console.log('updating morris');
			morris.setData(data.data);
		}
		else
			console.log('not updating morris');
	};

	NETDATA.morrisChartCreate = function(event, data) {
		var self = $(this);
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		var chart = self.data('chart');
		
		self.html('<div id="morris-' + chart.id + '" style="width: ' + width + 'px; height: ' + height + 'px;"></div>');

		// remove the 'time' element from the labels
		data.labels.splice(0, 1);

		var options = {
				element: 'morris-' + chart.id,
				data: data.data,
				xkey: 'time',
				ykeys: data.labels,
				labels: data.labels,
				lineWidth: 2,
				pointSize: 2,
				smooth: true,
				hideHover: 'auto',
				parseTime: true,
				continuousLine: false,
				behaveLikeLine: false,
				width: width,
				height: height
		};

		var morris;
		if(chart.chart_type == 'line')
			morris = new Morris.Line(options);

		else if(chart.chart_type == 'area') {
			options.behaveLikeLine = true;
			morris = new Morris.Area(options);
		}
		else // stacked
			morris = new Morris.Area(options);

		self.data('morris-instance', morris)
		.data('created', true);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// raphael

	NETDATA.raphaelInitialize = function(callback) {
		if(typeof netdataStopRaphael == 'undefined') {
			$.getScript(NETDATA.raphael_js)
				.done(function() {
					NETDATA.registerChartLibrary('raphael', NETDATA.raphael_js, 'json', 'flip|min2max', 1);
				})
				.fail(function() {
					NETDATA.error(100, NETDATA.raphael_js);
				})
				.always(function() {
					if(typeof callback == "function")
						callback();
				})
		}
		else if(typeof callback == "function")
			callback();
	};

	NETDATA.raphaelChartUpdate = function(event, data) {
		var self = $(this);
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		
		self.raphael(data, {
			width: width,
			height: height
		})
	};

	NETDATA.raphaelChartCreate = function(event, data) {
		var self = $(this);
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		
		self.raphael(data, {
			width: width,
			height: height
		})
		.data('created', true);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// google charts

	NETDATA.googleInitialize = function(callback) {
		if(typeof netdataStopGoogleCharts == 'undefined' && typeof google != 'undefined') {
			NETDATA.registerChartLibrary('google', NETDATA.google_js, 'datatable', '', 3);
			if(typeof callback == "function")
				callback();
		}
		else if(typeof callback == "function")
			callback();
	};

	NETDATA.googleChartUpdate = function(event, data) {
		var self = $(this);
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		var gchart = self.data('google-instance');
		var options = self.data('google-options');

		if(typeof data.update_every != 'undefined')
			self.data('update-every', data.update_every * 1000);

		var datatable = new google.visualization.DataTable(data);

		gchart.draw(datatable, options);
	};

	NETDATA.googleChartCreate = function(event, data) {
		var self = $(this);
		var width = self.data('width') || NETDATA.chartDefaults.width;
		var height = self.data('height') || NETDATA.chartDefaults.height;
		var chart = self.data('chart');
		
		var datatable = new google.visualization.DataTable(data);
		var gchart;

		var options = {
			width: width,
			height: height,
			lineWidth: 1,
			title: chart.title,
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
				title: chart.units,
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

		switch(chart.chart_type) {
			case "area":
				options.vAxis.viewWindowMode = 'maximized';
				gchart = new google.visualization.AreaChart(this);
				break;

			case "stacked":
				options.isStacked = true;
				options.areaOpacity = 0.85;
				options.vAxis.viewWindowMode = 'maximized';
				options.vAxis.minValue = null;
				options.vAxis.maxValue = null;
				gchart = new google.visualization.AreaChart(this);
				break;

			default:
			case "line":
				options.lineWidth = 2;
				gchart = new google.visualization.LineChart(this);
				break;
		}

		gchart.draw(datatable, options);

		self.data('google-instance', gchart)
		.data('google-options', options)
		.data('created', true);
	};

	// ----------------------------------------------------------------------------------------------------------------
	// load all libraries and initialize

	NETDATA.errorReset();

	NETDATA._loadjQuery(function() {		
		NETDATA.raphaelInitialize(function() {
			NETDATA.morrisInitialize(function() {
				NETDATA.peityInitialize(function() {
					NETDATA.sparklineInitialize(function() {
						NETDATA.dygraphInitialize(function() {
							NETDATA.googleInitialize(function() {
								netdataParsePageCharts();
							}) // google
						}) // dygraph.js
					}) // sparkline.js
				}) // piety.js
			}) // morris.js
		}) // raphael.js
	});

})(window);

