var page_is_visible = 1;

var TARGET_THUMB_GRAPH_WIDTH = 500;		// thumb charts width will range from 0.5 to 1.5 of that
var MINIMUM_THUMB_GRAPH_WIDTH = 400;	// thumb chart will generally try to be wider than that
var TARGET_THUMB_GRAPH_HEIGHT = 160;	// the height of the thumb charts
var TARGET_GROUP_GRAPH_HEIGHT = 160;

var THUMBS_MAX_TIME_TO_SHOW = 240;		// how much time the thumb charts will present?
var THUMBS_POINTS_DIVISOR = 3;
var THUMBS_STACKED_POINTS_DIVISOR = 4;

var GROUPS_MAX_TIME_TO_SHOW = 600;		// how much time the group charts will present?
var GROUPS_POINTS_DIVISOR = 2;
var GROUPS_STACKED_POINTS_DIVISOR = 3;

var MAINCHART_MIN_TIME_TO_SHOW = 1200;	// how much time the main chart will present by default?
var MAINCHART_POINTS_DIVISOR = 2;		// how much detailed will the main chart be by default? 1 = finest, higher is faster
var MAINCHART_STACKED_POINTS_DIVISOR = 3;		// how much detailed will the main chart be by default? 1 = finest, higher is faster

var MAINCHART_CONTROL_HEIGHT = 75;		// how tall the control chart will be
var MAINCHART_CONTROL_DIVISOR = 5;		// how much detailed will the control chart be? 1 = finest, higher is faster
var MAINCHART_INITIAL_SELECTOR= 20;		// 1/20th of the width, this overrides MAINCHART_MIN_TIME_TO_SHOW

var CHARTS_REFRESH_LOOP = 50;			// delay between chart refreshes
var CHARTS_REFRESH_IDLE = 500;			// delay between chart refreshes when no chart was ready for refresh the last time
var CHARTS_CHECK_NO_FOCUS = 500;		// delay to check for visibility when the page has no focus
var CHARTS_SCROLL_IDLE = 100;			// delay to wait after a page scroll

var ENABLE_CURVE = 1;

var resize_request = false;

function setPresentationNormal(ui) {
	THUMBS_POINTS_DIVISOR 				= 3;
	THUMBS_STACKED_POINTS_DIVISOR 		= Math.round(THUMBS_POINTS_DIVISOR * 1.5);
	GROUPS_POINTS_DIVISOR 				= 2;
	GROUPS_STACKED_POINTS_DIVISOR 		= Math.round(GROUPS_POINTS_DIVISOR * 1.5);
	MAINCHART_POINTS_DIVISOR 			= 2;
	MAINCHART_STACKED_POINTS_DIVISOR 	= Math.round(MAINCHART_POINTS_DIVISOR * 1.5);
	ENABLE_CURVE = 1;
	CHARTS_REFRESH_LOOP = 50;
	CHARTS_SCROLL_IDLE = 50;
	resize_request = true;
	if(ui) $('#presentation_normal').trigger('click');
	playGraphs();
}
function setPresentationSpeedy(ui) {
	THUMBS_POINTS_DIVISOR 				= 10;
	THUMBS_STACKED_POINTS_DIVISOR 		= Math.round(THUMBS_POINTS_DIVISOR * 1.5);
	GROUPS_POINTS_DIVISOR 				= 8;
	GROUPS_STACKED_POINTS_DIVISOR 		= Math.round(GROUPS_POINTS_DIVISOR * 1.5);
	MAINCHART_POINTS_DIVISOR 			= 5;
	MAINCHART_STACKED_POINTS_DIVISOR 	= Math.round(MAINCHART_POINTS_DIVISOR * 1.5);
	ENABLE_CURVE = 0;
	CHARTS_REFRESH_LOOP = 50;
	CHARTS_SCROLL_IDLE = 100;
	resize_request = true;
	if(ui) $('#presentation_speedy').trigger('click');
	playGraphs();
}
function setPresentationDetailed(ui) {
	THUMBS_POINTS_DIVISOR 				= 1;
	THUMBS_STACKED_POINTS_DIVISOR 		= 1;
	GROUPS_POINTS_DIVISOR 				= 1;
	GROUPS_STACKED_POINTS_DIVISOR 		= 1;
	MAINCHART_POINTS_DIVISOR 			= 1;
	MAINCHART_STACKED_POINTS_DIVISOR 	= 1;
	ENABLE_CURVE = 1;
	CHARTS_REFRESH_LOOP = 50;
	CHARTS_SCROLL_IDLE = 50;
	resize_request = true;
	if(ui) $('#presentation_detailed').trigger('click');
	playGraphs();
}

function isIE() {
  userAgent = navigator.userAgent;
  return userAgent.indexOf("MSIE ") > -1 || userAgent.indexOf("Trident/") > -1;
}

if(isIE()){
	// do stuff with ie-users
	CHARTS_REFRESH_LOOP=250;
	CHARTS_SCROLL_IDLE=500;
}

var MODE_THUMBS = 1;
var MODE_MAIN = 2;
var MODE_GROUP_THUMBS = 3;
var mode; // one of the MODE_* values

var allCharts = new Array();
var mainchart;

// html for the main menu
var mainmenu = "";
var categoriesmainmenu = "";
var familiesmainmenu = "";
var chartsmainmenu = "";


// ------------------------------------------------------------------------
// common HTML generation

function thumbChartActions(i, c, nogroup) {
	var name = c.name;
	if(!nogroup) name = c.family;

	var refinfo = "the chart is drawing ";
	if(c.group == 1) refinfo += "every single point collected (" + c.update_every + "s each).";
	else refinfo += ((c.group_method == "average")?"the average":"the max") + " value for every " + (c.group * c.update_every) + " seconds of data";

	var html = "<div class=\"btn-group btn-group\" data-toggle=\"tooltip\" data-placement=\"top\" title=\"" + refinfo + "\">"
	+		"<button type=\"button\" class=\"btn btn-default\" onclick=\"javascript: return;\"><span class=\"glyphicon glyphicon-info-sign\"></span></button>"
	+	"</div>"
	+	"<div class=\"btn-group btn-group\"><button type=\"button\" class=\"btn btn-default disabled\"><small>&nbsp;&nbsp; " + name + "</small></button>";

	if(!nogroup) {
		var ingroup = 0;
		var ingroup_detail = 0;

		$.each(allCharts, function(i, d) {
			if(d.family == c.family) {
				ingroup++;
				if(d.isdetail) ingroup_detail++;
			}
		});

		var hidden = "";
		if(ingroup_detail) hidden = ", including " + ingroup_detail + " charts not shown now";

		html += "<button type=\"button\" data-toggle=\"tooltip\" data-placement=\"top\" title=\"Show all " + ingroup + " charts in group '" + c.family + "'" + hidden + "\" class=\"btn btn-default\" onclick=\"initGroupGraphs('" + c.family +"');\"><span class=\"glyphicon glyphicon-th-large\"></span></button>";
	}

	html += "<button type=\"button\" data-toggle=\"tooltip\" data-placement=\"top\" title=\"show chart '" + c.name + "' in fullscreen\" class=\"btn btn-default\" onclick=\"initMainChartIndex(" + i +");\"><span class=\"glyphicon glyphicon-resize-full\"></span></button>"
	+		"<button type=\"button\" data-toggle=\"tooltip\" data-placement=\"top\" title=\"set options for chart '" + c.name + "'\" class=\"btn btn-default disabled\" onclick=\"alert('Not implemented yet!');\"><span class=\"glyphicon glyphicon-cog\"></span></button>"
	+		"<button type=\"button\" data-toggle=\"tooltip\" data-placement=\"top\" title=\"ignore chart '" + c.name + "'\" class=\"btn btn-default\" onclick=\"disableChart(" + i + ");\"><span class=\"glyphicon glyphicon-trash\"></span></button>"
	+	"</div>";

	return html;
}

function groupChartActions(i, c) {
	var name = c.name;

	var refinfo = "the chart is drawing ";
	if(c.group == 1) refinfo += "every single point collected (" + c.update_every + "s each).";
	else refinfo += ((c.group_method == "average")?"the average":"the max") + " value for every " + (c.group * c.update_every) + " seconds of data";

	var html = "<div class=\"btn-group btn-group\" data-toggle=\"tooltip\" data-placement=\"left\" title=\"" + refinfo + "\">"
	+		"<button type=\"button\" class=\"btn btn-default\" onclick=\"javascript: return;\"><span class=\"glyphicon glyphicon-info-sign\"></span></button>"
	+	"</div>";

	html += "<button type=\"button\" data-toggle=\"tooltip\" data-placement=\"left\" title=\"show chart '" + c.name + "' in fullscreen\" class=\"btn btn-default\" onclick=\"initMainChartIndex(" + i +");\"><span class=\"glyphicon glyphicon-resize-full\"></span></button>"
	+		"<button type=\"button\" data-toggle=\"tooltip\" data-placement=\"left\" title=\"ignore chart '" + c.name + "'\" class=\"btn btn-default\" onclick=\"disableChart(" + i + ");\"><span class=\"glyphicon glyphicon-trash\"></span></button>"
	+	"</div>";

	return html;
}

function mylog(txt) {
	console.log(txt);
	$('#logline').html(txt);
}

function chartssort(a, b) {
	if(a.priority == b.priority) {
		if(a.name < b.name) return -1;
	}
	else if(a.priority < b.priority) return -1;

	return 1;
}


// ------------------------------------------------------------------------
// MAINGRAPH = fullscreen view of 1 graph

// copy the chart c to mainchart
// switch to main graphs screen
function initMainChart(c) {
	if(mainchart) cleanThisChart(mainchart);

	mainchart = $.extend(true, {}, c);
	mainchart.enabled = true;
	mainchart.refreshCount = 0;
	mainchart.last_updated = 0;
	mainchart.chartOptions.explorer = null;
	mainchart.chart = null;

	mainchart.before = 0;
	mainchart.after = 0;

	mainchart.chartOptions.width = screenWidth();
	mainchart.chartOptions.height = $(window).height() - 150 - MAINCHART_CONTROL_HEIGHT;
	if(mainchart.chartOptions.height < 300) mainchart.chartOptions.height = 300;

	mainchart.div = 'maingraph';
	mainchart.max_time_to_show = (mainchart.last_entry_t - mainchart.first_entry_t) / ( MAINCHART_INITIAL_SELECTOR * mainchart.update_every );
	if(mainchart.max_time_to_show < MAINCHART_MIN_TIME_TO_SHOW) mainchart.max_time_to_show = MAINCHART_MIN_TIME_TO_SHOW;
	calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, mainchart.max_time_to_show, 0, ENABLE_CURVE);

	// copy it to the hidden chart
	mainchart.hiddenchart = $.extend(true, {}, mainchart);
	mainchart.hiddenchart.chartOptions.height = MAINCHART_CONTROL_HEIGHT;
	mainchart.hiddenchart.div = 'maingraph_control';
	mainchart.hiddenchart.non_zero = 0;

	// initialize the div
	showChartIsLoading(mainchart.div, mainchart.name, mainchart.chartOptions.width, mainchart.chartOptions.height);
	document.getElementById(mainchart.hiddenchart.div).innerHTML = "<table><tr><td align=\"center\" width=\"" + mainchart.hiddenchart.chartOptions.width + "\" height=\"" + mainchart.hiddenchart.chartOptions.height + "\" style=\"vertical-align:middle\"><h4><span class=\"label label-default\">Please wait...</span></h4></td></tr></table>";
	//showChartIsLoading(mainchart.hiddenchart.div, mainchart.hiddenchart.name, mainchart.hiddenchart.chartOptions.width, mainchart.hiddenchart.chartOptions.height);

	// set the radio buttons
	setMainChartGroupMethod(mainchart.group_method, 'no-refresh');
	setMainChartMax('normal');

	$('#group' + mainchart.group).trigger('click');
	setMainChartGroup(mainchart.group, 'no-refresh');

	switchToMainGraph();
}

function refreshHiddenChart(doNext) {
	if(refresh_mode == REFRESH_PAUSED && mainchart.hiddenchart.last_updated != 0) {
		if(typeof doNext == "function") doNext();
		return;
	}

	// is it too soon for a refresh?
	var now = Date.now();
	if((now - mainchart.hiddenchart.last_updated) < (mainchart.update_every * 10 * 1000) || (now - mainchart.hiddenchart.last_updated) < (mainchart.hiddenchart.group * mainchart.hiddenchart.update_every * 1000)) {
		if(typeof doNext == "function") doNext();
		return;
	}

	if(mainchart.dashboard && mainchart.hiddenchart.refreshCount > 50) {
		mainchart.dashboard.clear();
		mainchart.control_wrapper.clear();
		mainchart.hidden_wrapper.clear();

		mainchart.dashboard = null;
		mainchart.control_wrapper = null;
		mainchart.hidden_wrapper = null;
		mainchart.hiddenchart.last_updated = 0;
	}

	if(!mainchart.dashboard) {
		var controlopts = $.extend(true, {}, mainchart.chartOptions, {
			lineWidth: 1,
			height: mainchart.hiddenchart.chartOptions.height,
			chartArea: {'width': '98%'},
			hAxis: {'baselineColor': 'none', viewWindowMode: 'maximized', gridlines: { count: 0 } },
			vAxis: {'title': null, gridlines: { count: 0 } },
		});

		mainchart.dashboard = new google.visualization.Dashboard(document.getElementById('maingraph_dashboard'));
		mainchart.control_wrapper = new google.visualization.ControlWrapper({
			controlType: 'ChartRangeFilter',
			containerId: 'maingraph_control',
			options: {
				filterColumnIndex: 0,
				ui: {
					chartType: mainchart.chartType,
					chartOptions: controlopts,
					minRangeSize: (mainchart.max_time_to_show * 1000) / MAINCHART_POINTS_DIVISOR,
				}
			},
		});
		mainchart.hidden_wrapper = new google.visualization.ChartWrapper({
			chartType: mainchart.chartType,
			containerId: 'maingraph_hidden',
			options: {
				isStacked: mainchart.chartOptions.isStacked,
				width: mainchart.hiddenchart.chartOptions.width,
				height: mainchart.hiddenchart.chartOptions.height,
				//chartArea: {'height': '80%', 'width': '100%'},
				//hAxis: {'slantedText': false},
				//legend: {'position': 'none'}
			},
		});

		mainchart.hiddenchart.refreshCount = 0;
	}

	// load the data for the control and the hidden wrappers
	// calculate the group and points to show for the control chart
	calculateChartPointsToShow(mainchart.hiddenchart, MAINCHART_CONTROL_DIVISOR, 0, -1, ENABLE_CURVE);

	$.ajax({
		url: generateChartURL(mainchart.hiddenchart),
		dataType:"json",
		cache: false
	})
	.done(function(jsondata) {
		if(!jsondata || jsondata.length == 0) return;

		mainchart.control_data = new google.visualization.DataTable(jsondata);

		if(mainchart.hiddenchart.last_updated == 0) {
			google.visualization.events.addListener(mainchart.control_wrapper, 'ready', mainchartControlReadyEvent);
			mainchart.dashboard.bind(mainchart.control_wrapper, mainchart.hidden_wrapper);
		}
		if(refresh_mode != REFRESH_PAUSED) {
			// console.log('mainchart.points_to_show: ' + mainchart.points_to_show + ', mainchart.group: ' + mainchart.group + ', mainchart.update_every: ' + mainchart.update_every);

			var start = now - (mainchart.points_to_show * mainchart.group * mainchart.update_every * 1000);
			var end = now;
			var min = MAINCHART_MIN_TIME_TO_SHOW * 1000;
			if(end - start < min) start = end - min;

			mainchart.control_wrapper.setState({range: {
				start: new Date(start),
				end: new Date(end)
			},
			ui: {
				minRangeSize: min
			}});
		}

		mainchart.dashboard.draw(mainchart.control_data);
		mainchart.hiddenchart.last_updated = Date.now();
		mainchart.hiddenchart.refreshCount++;
	})
	.always(function() {
		if(typeof doNext == "function") doNext();
	});
}

function mainchartControlReadyEvent() {
	google.visualization.events.addListener(mainchart.control_wrapper, 'statechange', mainchartControlStateHandler);
	//mylog(mainchart);
}

function mainchartControlStateHandler() {
	// setMainChartPlay('pause');

	var state = mainchart.control_wrapper.getState();
	mainchart.after = Math.round(state.range.start.getTime() / 1000);
	mainchart.before = Math.round(state.range.end.getTime() / 1000);

	calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, mainchart.before - mainchart.after, 0, ENABLE_CURVE);
	//mylog('group = ' + mainchart.group + ', points_to_show = ' + mainchart.points_to_show + ', dt = ' + (mainchart.before - mainchart.after));

	$('#group' + mainchart.group).trigger('click');
	mainchart.last_updated = 0;

	if(refresh_mode != REFRESH_PAUSED) pauseGraphs();
}

function initMainChartIndex(i) {
	if(mode == MODE_GROUP_THUMBS)
		initMainChart(groupCharts[i]);

	else if(mode == MODE_THUMBS)
		initMainChart(allCharts[i]);

	else
		initMainChart(allCharts[i]);
}

function initMainChartIndexOfMyCharts(i) {
	initMainChart(allCharts[i]);
}

var last_main_chart_max='normal';
function setMainChartMax(m) {
	if(!mainchart) return;

	if(m == 'toggle') {
		if(last_main_chart_max == 'maximized') m = 'normal';
		else m = 'maximized';
	}

	if(m == "maximized") {
		mainchart.chartOptions.theme = 'maximized';
		//mainchart.chartOptions.axisTitlesPosition = 'in';
		//mainchart.chartOptions.legend = {position: 'none'};
		mainchart.chartOptions.hAxis.title = null;
		mainchart.chartOptions.hAxis.viewWindowMode = 'maximized';
		mainchart.chartOptions.vAxis.viewWindowMode = 'maximized';
		mainchart.chartOptions.chartArea = {'width': '98%', 'height': '100%'};
	}
	else {
		mainchart.chartOptions.hAxis.title = null;
		mainchart.chartOptions.theme = null;
		mainchart.chartOptions.hAxis.viewWindowMode = null;
		mainchart.chartOptions.vAxis.viewWindowMode = null;
		mainchart.chartOptions.chartArea = {'width': '80%', 'height': '90%'};
	}
	$('.mainchart_max_button').button(m);
	last_main_chart_max = m;
	mainchart.last_updated = 0;
}

function setMainChartGroup(g, norefresh) {
	if(!mainchart) return;

	mainchart.group = g;

	if(!mainchart.before && !mainchart.after)
		calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, mainchart.max_time_to_show, mainchart.group, ENABLE_CURVE);
	else
		calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, 0, mainchart.group, ENABLE_CURVE);

	if(!norefresh) {
		mainchart.last_updated = 0;
	}
}

var last_main_chart_avg = null;
function setMainChartGroupMethod(g, norefresh) {
	if(!mainchart) return;

	if(g == 'toggle') {
		if(last_main_chart_avg == 'max') g = 'average';
		else g = 'max';
	}

	mainchart.group_method = g;

	$('.mainchart_avg_button').button(g);

	if(!norefresh) {
		mainchart.last_updated = 0;
	}

	last_main_chart_avg = g;
}

function setMainChartPlay(p) {
	if(!mainchart) return;

	if(p == 'toggle') {
		if(refresh_mode != REFRESH_ALWAYS) p = 'play';
		else p = 'pause';
	}

	if(p == 'play') {
		//mainchart.chartOptions.explorer = null;
		mainchart.after = 0;
		mainchart.before = 0;
		calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, mainchart.max_time_to_show, 0, ENABLE_CURVE);
		$('#group' + mainchart.group).trigger('click');
		mainchart.last_updated = 0;
		mainchart.hiddenchart.last_updated = 0;
		playGraphs();
	}
	else {
		//mainchart.chartOptions.explorer = {
		//	'axis': 'horizontal',
		//	'maxZoomOut': 1,
		//};
		//mainchart.last_updated = 0;

		//if(!renderChart(mainchart, pauseGraphs))
		pauseGraphs();
	}
}

function buttonGlobalPlayPause(p) {
	if(mode == MODE_MAIN) {
		setMainChartPlay(p);
		return;
	}

	if(p == 'toggle') {
		if(refresh_mode != REFRESH_ALWAYS) p = 'play';
		else p = 'pause';
	}

	if(p == 'play') playGraphs();
	else pauseGraphs();
}


// ------------------------------------------------------------------------
// Chart resizing

function screenWidth() {
	return (($(window).width() * 0.95) - 50);
}

// calculate the proper width for the thumb charts
function thumbWidth() {
	var cwidth = screenWidth();
	var items = Math.round(cwidth / TARGET_THUMB_GRAPH_WIDTH);
	if(items < 1) items = 1;

	if(items > 1 && (cwidth / items) < MINIMUM_THUMB_GRAPH_WIDTH) items--;

	return Math.round(cwidth / items) - 1;
}

function groupChartSizes() {
	var s = { width: screenWidth(), height: TARGET_GROUP_GRAPH_HEIGHT };

	var count = 0;
	if(groupCharts) $.each(groupCharts, function(i, c) {
		if(c.enabled) count++;
	});

	if(count == 0) {
		s.width = TARGET_GROUP_GRAPH_HEIGHT;
		s.height = TARGET_GROUP_GRAPH_HEIGHT;
	}
	else {
		if(s.width < MINIMUM_THUMB_GRAPH_WIDTH) s.width = screenWidth();
		s.height = ($(window).height() - 130) / count - 10;
	}

	if(s.height < TARGET_GROUP_GRAPH_HEIGHT)
		s.height = TARGET_GROUP_GRAPH_HEIGHT;

	return s;
}

// resize all charts
// if the thumb charts need resize in their width, reset them
function resizeCharts() {
	var width = screenWidth();

	if(mainchart) {
		mainchart.chartOptions.width = width;
		mainchart.chartOptions.height = $(window).height() - 150 - MAINCHART_CONTROL_HEIGHT;
		mainchart.last_updated = 0;

		mainchart.hidden_wrapper.setOption('width', width);
		mainchart.control_wrapper.setOption('ui.chartOptions.width', width);
		mainchart.hiddenchart.chartOptions.width = width;
		mainchart.hiddenchart.last_updated = 0;
	}

	width = thumbWidth();
	$.each(allCharts, function(i, c) {
		if(c.enabled) {
			cleanThisChart(c);
			c.chartOptions.width = width;
			calculateChartPointsToShow(c, c.chartOptions.isStacked?THUMBS_STACKED_POINTS_DIVISOR:THUMBS_POINTS_DIVISOR, THUMBS_MAX_TIME_TO_SHOW, -1, ENABLE_CURVE);
			showChartIsLoading(c.div, c.name, c.chartOptions.width, c.chartOptions.height);
			document.getElementById(c.id + '_thumb_actions_div').innerHTML = thumbChartActions(i, c);
			c.last_updated = 0;
		}
	});

	if(groupCharts) $.each(groupCharts, function(i, c) {
		var sizes = groupChartSizes();

		if(c.enabled) {
			cleanThisChart(c);
			c.chartOptions.width = sizes.width;
			c.chartOptions.height = sizes.height;
			calculateChartPointsToShow(c, c.chartOptions.isStacked?GROUPS_STACKED_POINTS_DIVISOR:GROUPS_POINTS_DIVISOR, GROUPS_MAX_TIME_TO_SHOW, -1, ENABLE_CURVE);
			showChartIsLoading(c.div, c.name, c.chartOptions.width, c.chartOptions.height);
			document.getElementById(c.id + '_group_actions_div').innerHTML = groupChartActions(i, c);
			c.last_updated = 0;
		}
	});

	updateUI();
}

window.onresize = function(event) {
	resize_request = true;
};


// ------------------------------------------------------------------------
// Core of the thread refreshing the charts

var REFRESH_PAUSED = 0;
var REFRESH_ALWAYS = 1;

var refresh_mode = REFRESH_PAUSED;
var last_refresh = 0;
function playGraphs() {
	mylog('playGraphs()');
	if(refresh_mode == REFRESH_ALWAYS) return;

	//mylog('PlayGraphs()');
	refresh_mode = REFRESH_ALWAYS;
	$('.mainchart_play_button').button('play');
	$('.global_play_button').button('play');

	// check if the thread died due to a javascript error
	var now = Date.now();
	if((now - last_refresh) > 60000) {
		// it died or never started
		//mylog('It seems the refresh thread died. Restarting it.');
		renderChartCallback();
	}
}

function pauseGraphs() {
	mylog('pauseGraphs()');
	if(refresh_mode == REFRESH_PAUSED) return;

	refresh_mode = REFRESH_PAUSED;
	$('.mainchart_play_button').button('pause');
	$('.global_play_button').button('pause');
}

var interval = null;
function checkRefreshThread() {
	if(interval == null) {
		interval = setInterval(checkRefreshThread, 2000);
		return;
	}

	var now = Date.now();
	if(now - last_refresh > 60000) {
		mylog('Refresh thread died. Restarting it.');
		renderChartCallback();
	}
}

// refresh the proper chart
// this is an internal function.
// never call it directly, or new javascript threads will be spawn
var timeout = null;
function renderChartCallback() {
	last_refresh = Date.now();

	if(!page_is_visible) {
		timeout = setTimeout(triggerRefresh, CHARTS_CHECK_NO_FOCUS);
		return;
	}

	if(resize_request) {
		mylog('renderChartCallback() resize_request is set');
		cleanupCharts();
		resizeCharts();
		resize_request = false;
		// refresh_mode = REFRESH_ALWAYS;
	}

	if(last_user_scroll) {
		var now = Date.now();
		if((now - last_user_scroll) >= CHARTS_SCROLL_IDLE) {
			last_user_scroll = 0;
			mylog('Scrolling: resuming refresh...');
		}
		else {
			mylog('Scrolling: pausing refresh for ' + (CHARTS_SCROLL_IDLE - (now - last_user_scroll)) + ' ms...');
			timeout = setTimeout(triggerRefresh, CHARTS_SCROLL_IDLE - (now - last_user_scroll));
			return;
		}
	}

	if(refresh_mode == REFRESH_PAUSED) {
		if(mode == MODE_MAIN && mainchart.last_updated == 0) {
			mainChartRefresh();
			return;
		}

		if(mode != MODE_MAIN) {
			timeout = setTimeout(triggerRefresh, CHARTS_REFRESH_IDLE);
			return;
		}
	}

	     if(mode == MODE_THUMBS)		timeout = setTimeout(thumbChartsRefreshNext, CHARTS_REFRESH_LOOP);
	else if(mode == MODE_GROUP_THUMBS)  timeout = setTimeout(groupChartsRefreshNext, CHARTS_REFRESH_LOOP);
	else if(mode == MODE_MAIN)   		timeout = setTimeout(mainChartRefresh, CHARTS_REFRESH_LOOP);
	else                         		timeout = setTimeout(triggerRefresh, CHARTS_REFRESH_IDLE);
}

// callback for refreshing the charts later
// this is an internal function.
// never call it directly, or new javascript threads will be spawn
function triggerRefresh() {
	//mylog('triggerRefresh()');

	if(!page_is_visible || (refresh_mode == REFRESH_PAUSED && mode != MODE_MAIN)) {
		last_refresh = Date.now();
		timeout = setTimeout(triggerRefresh, CHARTS_REFRESH_IDLE);
		return;
	}

	     if(mode == MODE_THUMBS) 		timeout = setTimeout(renderChartCallback, CHARTS_REFRESH_IDLE);
	else if(mode == MODE_GROUP_THUMBS)	timeout = setTimeout(renderChartCallback, CHARTS_REFRESH_IDLE);
	else if(mode == MODE_MAIN)   		timeout = setTimeout(renderChartCallback, CHARTS_REFRESH_IDLE);
	else                         		timeout = setTimeout(triggerRefresh, CHARTS_REFRESH_IDLE);
}

// refresh the main chart
// make sure we don't loose the refreshing thread
function mainChartRefresh() {
	//mylog('mainChartRefresh()');

	if(mode != MODE_MAIN || !mainchart) {
		triggerRefresh();
		return;
	}

	if(refresh_mode == REFRESH_PAUSED && mainchart.last_updated != 0) {
		hiddenChartRefresh();
		return;
	}

	if(!renderChart(mainchart, hiddenChartRefresh))
		hiddenChartRefresh();
}

function hiddenChartRefresh() {
	refreshHiddenChart(triggerRefresh);
}

function roundRobinRenderChart(charts, startat) {
	var refreshed = false;

	// find a chart to refresh
	var all = charts.length;
	var cur = startat + 1;
	var count = 0;

	for(count = 0; count < all ; count++, cur++) {
		if(cur >= all) cur = 0;

		if(charts[cur].enabled) {
			refreshed = renderChart(charts[cur], renderChartCallback);
			if(refreshed) {
				mylog('Refreshed: ' + charts[cur].name);
				break;
			}
		}
	}

	if(!refreshed) triggerRefresh();
	return cur;
}

// refresh the thumb charts
// make sure we don't loose the refreshing thread
var last_thumb_updated = 0;
function thumbChartsRefreshNext() {
	//mylog('thumbChartsRefreshNext()');

	if(allCharts.length == 0 || mode != MODE_THUMBS) {
		triggerRefresh();
		return;
	}

	last_thumb_updated = roundRobinRenderChart(allCharts, last_thumb_updated);
}

// refresh the group charts
// make sure we don't loose the refreshing thread
var last_group_updated = 0;
function groupChartsRefreshNext() {
	//mylog('groupChartsRefreshNext()');

	if(!groupCharts || groupCharts.length == 0 || mode != MODE_GROUP_THUMBS) {
		//mylog('cannot refresh charts');
		triggerRefresh();
		return;
	}

	last_group_updated = roundRobinRenderChart(groupCharts, last_group_updated);
}


// ------------------------------------------------------------------------
// switch the screen between views
// these should be called only from initXXXX()

function disableChart(i) {
	mylog('disableChart(' + i + ')');

	var chart = null;

	var count = 0;
	if(mode == MODE_GROUP_THUMBS && groupCharts) {
		$.each(groupCharts, function(i, c) {
			if(c.enabled) count++;
		});

		if(i < groupCharts.length) chart = groupCharts[i];
	}
	else if(mode == MODE_THUMBS) {
		$.each(allCharts, function(i, c) {
			if(c.enabled) count++;
		});

		if(i < allCharts.length) chart = allCharts[i];
	}

	if(!chart) return;

	if(count <= 1) {
		alert('Cannot close the last chart shown.');
		return;
	}

	if(chart) {
		mylog("request to disable chart " + chart.name);
		chart.disablethisplease = true;
		resize_request = true;
	}
	else
		mylog("no chart to disable");
}

function cleanThisChart(chart, emptydivs) {
	//mylog('cleanThisChart(' + chart.name + ', ' + emptydivs +')');

	if(chart.dashboard) {
		chart.dashboard.clear();
		chart.dashboard = null;

		if(chart.control_wrapper) {
			chart.control_wrapper.clear();
			chart.control_wrapper = null;
		}

		if(chart.hidden_wrapper) {
			chart.hidden_wrapper.clear();
			chart.hidden_wrapper = null;
		}

		chart.control_data = null;
	}

	if(chart.chart) chart.chart.clearChart();
	chart.chart = null;

	if(emptydivs) {
		var div = document.getElementById(chart.div);
		if(div) {
			div.style.display = 'none';
			div.innerHTML = "";
		}

		div = document.getElementById(chart.div + "_parent");
		if(div) {
			div.style.display = 'none';
			div.innerHTML = "";
		}
	}

	//mylog("chart " + chart.name + " cleaned with option " + emptydivs);
}

// cleanup the previously shown charts
function cleanupCharts() {
	// mylog('cleanupCharts()');

	if(mode != MODE_MAIN && mainchart) {
		if(mainchart.chart) cleanThisChart(mainchart);
		mainchart = null;
	}

	if(mode != MODE_GROUP_THUMBS && groupCharts) {
		clearGroupGraphs();
	}

	// cleanup the disabled charts
	$.each(allCharts, function(i, c) {
		if(c.disablethisplease && c.enabled) {
			cleanThisChart(c, 'emptydivs');
			c.disablethisplease = false;
			c.enabled = false;
			resize_request = true;
			mylog("removed thumb chart " + c.name + " removed");
		}
	});

	if(groupCharts) $.each(groupCharts, function(i, c) {
		if(c.disablethisplease && c.enabled) {
			cleanThisChart(c, 'emptydivs');
			c.disablethisplease = false;
			c.enabled = false;
			resize_request = true;
			mylog("removed group chart " + c.name + " removed");
		}
	});

	// we never cleanup the main chart
}

function updateUI() {
	$('[data-toggle="tooltip"]').tooltip({'container': 'body', 'html': true});

	$('[data-spy="scroll"]').each(function () {
		var $spy = $(this).scrollspy('refresh')
	})
}

var thumbsScrollPosition = null;
function switchToMainGraph() {
	//mylog('switchToMainGraph()');

	if(!mainchart) return;

	if(!groupCharts) thumbsScrollPosition = window.pageYOffset;

	document.getElementById('maingraph_container').style.display = 'block';
	document.getElementById('thumbgraphs_container').style.display = 'none';
	document.getElementById('groupgraphs_container').style.display = 'none';
	document.getElementById('splash_container').style.display = 'none';

	document.getElementById("main_menu_div").innerHTML = "<ul class=\"nav navbar-nav\"><li><a href=\"javascript:switchToThumbGraphs();\"><span class=\"glyphicon glyphicon-circle-arrow-left\"></span> Back to Dashboard</a></li><li class=\"active\"><a href=\"#\">" + mainchart.name + "</a></li>" + familiesmainmenu + chartsmainmenu + "</ul>" ;

	window.scrollTo(0, 0);

	mode = MODE_MAIN;
	playGraphs();
	updateUI();
}

function switchToThumbGraphs() {
	//mylog('switchToThumbGraphs()');

	document.getElementById('maingraph_container').style.display = 'none';
	document.getElementById('thumbgraphs_container').style.display = 'block';
	document.getElementById('groupgraphs_container').style.display = 'none';
	document.getElementById('splash_container').style.display = 'none';

	document.getElementById("main_menu_div").innerHTML = mainmenu;

	if(thumbsScrollPosition) window.scrollTo(0, thumbsScrollPosition);

	// switch mode
	mode = MODE_THUMBS;
	playGraphs();
	updateUI();
}

function switchToGroupGraphs() {
	//mylog('switchToGroupGraphs()');

	if(!groupCharts) return;

	if(!mainchart) thumbsScrollPosition = window.pageYOffset;

	document.getElementById('maingraph_container').style.display = 'none';
	document.getElementById('thumbgraphs_container').style.display = 'none';
	document.getElementById('groupgraphs_container').style.display = 'block';
	document.getElementById('splash_container').style.display = 'none';

	document.getElementById("main_menu_div").innerHTML = "<ul class=\"nav navbar-nav\"><li><a href=\"javascript:switchToThumbGraphs();\"><span class=\"glyphicon glyphicon-circle-arrow-left\"></span> Back to Dashboard</a></li><li class=\"active\"><a href=\"#\">" + groupCharts[0].family + "</a></li>" + familiesmainmenu + chartsmainmenu + "</ul>";

	window.scrollTo(0, 0);

	mode = MODE_GROUP_THUMBS;
	playGraphs();
	updateUI();
}


// ------------------------------------------------------------------------
// Group Charts

var groupCharts = null;
function initGroupGraphs(group) {
	var count = 0;

	if(groupCharts) clearGroupGraphs();
	groupCharts = new Array();

	var groupbody = "";
	$.each(allCharts, function(i, c) {
		if(c.family == group) {
			groupCharts[count] = [];
			groupCharts[count] = $.extend(true, {}, c);
			groupCharts[count].div += "_group";
			groupCharts[count].enabled = true;
			groupCharts[count].chart = null;
			groupCharts[count].last_updated = 0;
			count++;
		}
	});
	groupCharts.sort(chartssort);

	var sizes = groupChartSizes();

	var groupbody = "";
	$.each(groupCharts, function(i, c) {
		c.chartOptions.width = sizes.width;
		c.chartOptions.height = sizes.height;
		c.chartOptions.chartArea.width = '85%';
		c.chartOptions.chartArea.height = '90%';
		c.chartOptions.hAxis.textPosition = 'in';
		c.chartOptions.hAxis.viewWindowMode = 'maximized';
		c.chartOptions.hAxis.textStyle = { "fontSize": 9 };
		c.chartOptions.vAxis.textStyle = { "fontSize": 9 };
		c.chartOptions.fontSize = 11;
		c.chartOptions.titlePosition = 'in';
		c.chartOptions.tooltip = { "textStyle": { "fontSize": 9 } };
		c.chartOptions.legend = { "textStyle": { "fontSize": 9 } };

		calculateChartPointsToShow(c, c.chartOptions.isStacked?GROUPS_STACKED_POINTS_DIVISOR:GROUPS_POINTS_DIVISOR, GROUPS_MAX_TIME_TO_SHOW, -1, ENABLE_CURVE);

		groupbody += "<div class=\"thumbgraph\" id=\"" + c.div + "_parent\"><table><tr><td width='" + sizes.width + "'><div class=\"thumbgraph\" id=\"" + c.div + "\">" + chartIsLoadingHTML(c.name, c.chartOptions.width, c.chartOptions.height) + "</div></td><td id=\"" + c.id + "_group_actions_div\" align=\"center\">" + groupChartActions(i, c) + "</td></tr><tr><td width='15'></td></tr></table></div>";
		//groupbody += "<div class=\"thumbgraph\" id=\"" + c.div + "\">" + chartIsLoadingHTML(c.name, c.chartOptions.width, c.chartOptions.height) + "</div>";
	});
	groupbody += "";

	document.getElementById("groupgraphs").innerHTML = groupbody;
	switchToGroupGraphs();
}

function clearGroupGraphs() {
	if(groupCharts && groupCharts.length) {
		$.each(groupCharts, function(i, c) {
			cleanThisChart(c, 'emptydivs');
		});

		groupCharts = null;
	}

	document.getElementById("groupgraphs").innerHTML = "";
}


// ------------------------------------------------------------------------
// Global entry point
// initialize the thumb charts

var last_user_scroll = 0;

// load the charts from the server
// generate html for the thumbgraphs to support them
function initCharts() {
	setPresentationNormal(1);

	var width = thumbWidth();
	var height = TARGET_THUMB_GRAPH_HEIGHT;

	window.onscroll = function (e) {
		last_user_scroll = Date.now();
		mylog('Scrolling: detected');
	}

	loadCharts(null, function(all) {
		allCharts = all.charts;

		if(allCharts == null || allCharts.length == 0) {
			alert("Cannot load data from server.");
			return;
		}

		var thumbsContainer = document.getElementById("thumbgraphs");
		if(!thumbsContainer) {
			alert("Cannot find the thumbsContainer");
			return;
		}

		allCharts.sort(chartssort);

		document.getElementById('hostname_id').innerHTML = all.hostname;
		document.title = all.hostname;

		// create an array for grouping all same-type graphs together
		var dimensions = 0;
		var categories = new Array();
		var families = new Array();
		var chartslist = new Array();
		$.each(allCharts, function(i, c) {
			var j;

			chartslist.push({name: c.name, type: c.type, id: i});

			dimensions += c.dimensions.length;
			c.chartOptions.width = width;
			c.chartOptions.height = height;

			// calculate how many point to show for each chart
			//c.points_to_show = Math.round(c.entries / c.group) - 1;
			// show max 10 mins of data
			//if(c.points_to_show * c.group > THUMBS_MAX_TIME_TO_SHOW) c.points_to_show = THUMBS_MAX_TIME_TO_SHOW / c.group;
			calculateChartPointsToShow(c, c.chartOptions.isStacked?THUMBS_STACKED_POINTS_DIVISOR:THUMBS_POINTS_DIVISOR, THUMBS_MAX_TIME_TO_SHOW, -1, ENABLE_CURVE);

			if(c.enabled) {
				var h = "<div class=\"thumbgraph\" id=\"" + c.div + "_parent\"><table><tr><td><div class=\"thumbgraph\" id=\"" + c.div + "\">" + chartIsLoadingHTML(c.name, c.chartOptions.width, c.chartOptions.height) + "</div></td></tr><tr><td id=\"" + c.id + "_thumb_actions_div\" align=\"center\">"
				+ thumbChartActions(i, c)
				+	"</td></tr><tr><td height='15'></td></tr></table></div>";

				// find the categories object for this type
				for(j = 0; j < categories.length ;j++) {
					if(categories[j].name == c.type) {
						categories[j].html += h;
						categories[j].count++;
						break;
					}
				}

				if(j == categories.length)
					categories.push({name: c.type, title: c.category, description: '', priority: c.categoryPriority, count: 1, glyphicon: c.glyphicon, html: h});
			}

			// find the families object for this type
			for(j = 0; j < families.length ;j++) {
				if(families[j].name == c.family) {
					families[j].count++;
					break;
				}
			}

			if(j == families.length)
				families.push({name: c.family, count: 1});
		});

		document.getElementById('server_summary_id').innerHTML = "<small>NetData server at <b>" + all.hostname + "</b> is maintaining <b>" + allCharts.length + "</b> charts, having <b>" + dimensions + "</b> dimensions (by default with <b>" + all.history + "</b> entries each), which are updated every <b>" + all.update_every + "s</b>, using a total of <b>" + (Math.round(all.memory * 10 / 1024 / 1024) / 10) + " MB</b> for the round robin database.</small>";

		$.each(categories, function(i, a) {
			a.html = "<tr><td id=\"" + a.name + "\"><ol class=\"breadcrumb graphs\"><li class=\"active\"><span class=\"glyphicon " + a.glyphicon + "\"></span> &nbsp; <a id=\"" + a.name + "\" href=\"#" + a.name + "\"><b>" + a.title + "</b> " + a.description + " </a></li></ol></td></tr><tr><td><div class=\"thumbgraphs\">" + a.html + "</td></tr>";
		});

		function categoriessort(a, b) {
			if(a.priority < b.priority) return -1;
			return 1;
		}
		categories.sort(categoriessort);

		function familiessort(a, b) {
			if(a.name < b.name) return -1;
			return 1;
		}
		families.sort(familiessort);

		function chartslistsort(a, b) {
			if(a.name < b.name) return -1;
			return 1;
		}
		chartslist.sort(chartslistsort);

		// combine all the htmls into one
		var allcategories = "<table width=\"100%\">";
		mainmenu = '<ul class="nav navbar-nav">';

		categoriesmainmenu = '<li class="dropdown"><a href="#" class="dropdown-toggle" data-toggle="dropdown"><span class=\"glyphicon glyphicon-fire\"></span> Dashboard Sections <b class="caret"></b></a><ul class="dropdown-menu">';
		$.each(categories, function(i, a) {
			allcategories += a.html;
			categoriesmainmenu += "<li><a href=\"#" + a.name + "\">" + a.title + "</a></li>";
		});
		categoriesmainmenu += "</ul></li>";
		allcategories += "</table>";

		familiesmainmenu = '<li class="dropdown"><a href="#" class="dropdown-toggle" data-toggle="dropdown"><span class=\"glyphicon glyphicon-th-large\"></span> Chart Families <b class="caret"></b></a><ul class="dropdown-menu">';
		$.each(families, function(i, a) {
			familiesmainmenu += "<li><a href=\"javascript:initGroupGraphs('" + a.name + "');\">" + a.name + " <span class=\"badge pull-right\">" + a.count + "</span></a></li>";
		});
		familiesmainmenu += "</ul></li>";

		chartsmainmenu = '<li class="dropdown"><a href="#" class="dropdown-toggle" data-toggle="dropdown"><span class=\"glyphicon glyphicon-resize-full\"></span> All Charts <b class="caret"></b></a><ul class="dropdown-menu">';
		$.each(chartslist, function(i, a) {
			chartsmainmenu += "<li><a href=\"javascript:initMainChartIndexOfMyCharts('" + a.id + "');\">" + a.name + "</a></li>";
		});
		chartsmainmenu += "</ul></li>";

		mainmenu += categoriesmainmenu;
		mainmenu += familiesmainmenu;
		mainmenu += chartsmainmenu;
		mainmenu += '<li role="presentation" class="disabled" style="display: none;"><a href="#" id="logline"></a></li></ul>';

		thumbsContainer.innerHTML = allcategories;
		switchToThumbGraphs();
		checkRefreshThread();
	});
}

$(window).blur(function() {
	page_is_visible = 0;
	mylog('Lost Focus!');
});

$(window).focus(function() {
	page_is_visible = 1;
	mylog('Focus restored!');
});

// Set a callback to run when the Google Visualization API is loaded.
google.setOnLoadCallback(initCharts);
