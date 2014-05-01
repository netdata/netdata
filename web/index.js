var TARGET_THUMB_GRAPH_WIDTH = 500;		// thumb charts width will range from 0.5 to 1.5 of that
var MINIMUM_THUMB_GRAPH_WIDTH = 400;	// thumb chart will generally try to be wider than that
var TARGET_THUMB_GRAPH_HEIGHT = 180;	// the height of the thumb charts

var THUMBS_MAX_TIME_TO_SHOW = 600;		// how much time the thumb charts will present?
var THUMBS_POINTS_DIVISOR = 4;
var THUMBS_STACKED_POINTS_DIVISOR = 10;

var GROUPS_MAX_TIME_TO_SHOW = 600;		// how much time the thumb charts will present?
var GROUPS_POINTS_DIVISOR = 4;
var GROUPS_STACKED_POINTS_DIVISOR = 10;

var MAINCHART_MAX_TIME_TO_SHOW = 600;	// how much time the main chart will present by default?
var MAINCHART_POINTS_DIVISOR = 10;		// how much detailed will the main chart be by default? 1 = finest, higher is faster
var MAINCHART_STACKED_POINTS_DIVISOR = 20;		// how much detailed will the main chart be by default? 1 = finest, higher is faster

var MAINCHART_CONTROL_HEIGHT = 75;		// how tall the control chart will be
var MAINCHART_CONTROL_DIVISOR = 2;		// how much detailed will the control chart be? 1 = finest, higher is faster

var MODE_THUMBS = 1;
var MODE_MAIN = 2;
var MODE_GROUP_THUMBS = 3;
var mode; // one of the MODE_* values

var mycharts = new Array();
var mainchart;

// html for the main menu
var mainmenu = "";


// ------------------------------------------------------------------------
// common HTML generation

function thumbChartActions(i, c, nogroup) {
	var name = c.name;
	if(!nogroup) name = c.family;

	var refinfo = "the chart is drawing ";
	if(c.group == 1) refinfo += "every single point collected (" + c.update_every + "s each).";
	else refinfo += ((c.group_method == "average")?"the average":"the max") + " value for every " + (c.group * c.update_every) + " seconds of data";

	var html = "<div class=\"btn-group btn-group\" data-toggle=\"tooltip\" title=\"" + refinfo + "\">"
	+		"<button type=\"button\" class=\"btn btn-default\" onclick=\"javascript: return;\"><span class=\"glyphicon glyphicon-info-sign\"></span></button>"
	+	"</div>"
	+	"<div class=\"btn-group btn-group\"><button type=\"button\" class=\"btn btn-default disabled\"><small>&nbsp;&nbsp; " + name + "</small></button>";

	if(!nogroup) {
		var ingroup = 0;
		var ingroup_detail = 0;

		$.each(mycharts, function(i, d) {
			if(d.family == c.family) {
				ingroup++;
				if(d.isdetail) ingroup_detail++;
			}
		});

		var hidden = "";
		if(ingroup_detail) hidden = ", including " + ingroup_detail + " charts not shown now";

		html += "<button type=\"button\" data-toggle=\"tooltip\" title=\"Show all " + ingroup + " charts in group '" + c.family + "'" + hidden + "\" class=\"btn btn-default\" onclick=\"initGroupGraphs('" + c.family +"');\"><span class=\"glyphicon glyphicon-th-large\"></span></button>";
	}

	html += "<button type=\"button\" data-toggle=\"tooltip\" title=\"show chart '" + c.name + "' in fullscreen\" class=\"btn btn-default\" onclick=\"initMainChartIndex(" + i +");\"><span class=\"glyphicon glyphicon-resize-full\"></span></button>"
	+		"<button type=\"button\" data-toggle=\"tooltip\" title=\"set options for chart '" + c.name + "'\" class=\"btn btn-default disabled\" onclick=\"alert('Not implemented yet!');\"><span class=\"glyphicon glyphicon-cog\"></span></button>"
	+		"<button type=\"button\" data-toggle=\"tooltip\" title=\"ignore chart '" + c.name + "'\" class=\"btn btn-default\" onclick=\"disableChart(" + i + ");\"><span class=\"glyphicon glyphicon-trash\"></span></button>"
	+	"</div>";

	return html;
}

function mylog(txt) {
	console.log(txt);
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
	mainchart.refreshCount = 0;
	mainchart.last_updated = 0;
	mainchart.chartOptions.explorer = null;
	mainchart.chart = null;

	mainchart.chartOptions.width = screenWidth();
	mainchart.chartOptions.height = $(window).height() - 150 - MAINCHART_CONTROL_HEIGHT;
	if(mainchart.chartOptions.height < 300) mainchart.chartOptions.height = 300;

	mainchart.div = 'maingraph';
	calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, MAINCHART_MAX_TIME_TO_SHOW, 0);

	// copy it to the hidden chart
	mainchart.hiddenchart = $.extend(true, {}, mainchart);
	mainchart.hiddenchart.chartOptions.height = MAINCHART_CONTROL_HEIGHT;
	mainchart.hiddenchart.div = 'maingraph_control';

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
	var now = new Date().getTime();
	if((now - mainchart.hiddenchart.last_updated) < (mainchart.hiddenchart.group * mainchart.hiddenchart.update_every * 1000)) {
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
			hAxis: {'baselineColor': 'none'},
			vAxis: {'title': null},
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
					minRangeSize: (MAINCHART_MAX_TIME_TO_SHOW * 1000 * mainchart.update_every) / MAINCHART_POINTS_DIVISOR,
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
	calculateChartPointsToShow(mainchart.hiddenchart, MAINCHART_CONTROL_DIVISOR, 0, -1);

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
		if(refresh_mode != REFRESH_PAUSED)
			mainchart.control_wrapper.setState({range: {
				start: new Date((Math.round(new Date().getTime() / 1000) - MAINCHART_MAX_TIME_TO_SHOW) * 1000),
				end: new Date()
			}});

		mainchart.dashboard.draw(mainchart.control_data);
		mainchart.hiddenchart.last_updated = new Date().getTime();
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

	calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, 0, 0);
	//mylog('group = ' + mainchart.group + ', points_to_show = ' + mainchart.points_to_show + ', dt = ' + (mainchart.before - mainchart.after));

	$('#group' + mainchart.group).trigger('click');
	mainchart.last_updated = 0;

	if(refresh_mode != REFRESH_PAUSED) pauseGraphs();
}

function initMainChartIndex(i) {
	if(mode == MODE_GROUP_THUMBS) 
		initMainChart(group_charts[i]);

	else if(mode == MODE_THUMBS)
		initMainChart(mycharts[i]);
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
		calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, MAINCHART_MAX_TIME_TO_SHOW, mainchart.group);
	else
		calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, 0, mainchart.group);

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
		calculateChartPointsToShow(mainchart, mainchart.chartOptions.isStacked?MAINCHART_STACKED_POINTS_DIVISOR:MAINCHART_POINTS_DIVISOR, MAINCHART_MAX_TIME_TO_SHOW, 0);
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
		
		//if(!refreshChart(mainchart, pauseGraphs))
		pauseGraphs();
	}
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
	var s = { width: screenWidth() / 2, height: ($(window).height() - 130) / 3 - 10};
	if(s.width < MINIMUM_THUMB_GRAPH_WIDTH * 1.5) s.width = screenWidth();

	var count = 0;
	if(group_charts) $.each(group_charts, function(i, c) {
		if(c.enabled) count++;
	});

	if(count == 0) {
		s.width = TARGET_THUMB_GRAPH_WIDTH;
		s.height = TARGET_THUMB_GRAPH_HEIGHT;
	}
	if(count < 4) {
		s.width = screenWidth();
		s.height = ($(window).height() - 130) / count - 10;
	}
	else if(count == 4) {
		s.height = ($(window).height() - 130) / 2 - 10;
	}

	if(s.height < TARGET_THUMB_GRAPH_HEIGHT * 1.5)
		s.height = TARGET_THUMB_GRAPH_HEIGHT * 1.5;

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
	$.each(mycharts, function(i, c) {
		if(c.enabled && c.chartOptions.width != width) {
			cleanThisChart(c);
			c.chartOptions.width = width;
			calculateChartPointsToShow(c, c.chartOptions.isStacked?THUMBS_STACKED_POINTS_DIVISOR:THUMBS_POINTS_DIVISOR, THUMBS_MAX_TIME_TO_SHOW, -1);
			showChartIsLoading(c.div, c.name, c.chartOptions.width, c.chartOptions.height);
			c.last_updated = 0;
		}
	});

	if(group_charts) $.each(group_charts, function(i, c) {
		var sizes = groupChartSizes();

		if(c.enabled && (c.chartOptions.width != sizes.width || c.chartOptions.height != sizes.height)) {
			cleanThisChart(c);
			c.chartOptions.width = sizes.width;
			c.chartOptions.height = sizes.height;
			calculateChartPointsToShow(c, c.chartOptions.isStacked?GROUPS_STACKED_POINTS_DIVISOR:GROUPS_POINTS_DIVISOR, GROUPS_MAX_TIME_TO_SHOW, -1);
			showChartIsLoading(c.div, c.name, c.chartOptions.width, c.chartOptions.height);
			c.last_updated = 0;
		}
	});
}

var resize_request = false;
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
	if(refresh_mode == REFRESH_ALWAYS) return;

	//mylog('PlayGraphs()');
	refresh_mode = REFRESH_ALWAYS;
	$('.mainchart_play_button').button('play');

	// check if the thread died due to a javascript error
	var now = new Date().getTime();
	if((now - last_refresh) > 5000) {
		// it died or never started
		//mylog('It seems the refresh thread died. Restarting it.');
		chartsRefresh();
	}
}

function pauseGraphs() {
	if(refresh_mode == REFRESH_PAUSED) return;

	//mylog('PauseGraphs()');
	refresh_mode = REFRESH_PAUSED;
	$('.mainchart_play_button').button('pause');
}

var interval = null;
function checkRefreshThread() {
	if(interval == null) {
		interval = setInterval(checkRefreshThread, 2000);
		return;
	}

	var now = new Date().getTime();
	if(now - last_refresh > 10000) {
		mylog('Refresh thread died. Restarting it.');
		chartsRefresh();
	}
}

// refresh the proper chart
// this is an internal function.
// never call it directly, or new javascript threads will be spawn
var timeout = null;
function chartsRefresh() {
	if(resize_request) {
		resizeCharts();
		resize_request = false;
		// refresh_mode = REFRESH_ALWAYS;
	}

	last_refresh = new Date().getTime();

	if(refresh_mode == REFRESH_PAUSED) {
		if(mode == MODE_MAIN && mainchart.last_updated == 0) {
			mainChartRefresh();
			return;
		}
	}

	     if(mode == MODE_THUMBS)		thumbChartsRefresh();
	else if(mode == MODE_GROUP_THUMBS)  groupChartsRefresh();
	else if(mode == MODE_MAIN)   		mainChartRefresh();
	else                         		timeout = setTimeout(triggerRefresh, 100);
}

// callback for refreshing the charts later
// this is an internal function.
// never call it directly, or new javascript threads will be spawn
function triggerRefresh() {
	//mylog('triggerRefresh()');

	// cleanup has to take place when the charts are not refreshed
	// since the refreshing thread is in this function, it means
	// nothing is being refreshed.
	cleanupCharts();

	     if(mode == MODE_THUMBS) 		timeout = setTimeout(chartsRefresh, 200);
	else if(mode == MODE_GROUP_THUMBS)	timeout = setTimeout(chartsRefresh, 200);
	else if(mode == MODE_MAIN)   		timeout = setTimeout(chartsRefresh, 200);
	else                         		timeout = setTimeout(triggerRefresh, 100);
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

	if(!refreshChart(mainchart, hiddenChartRefresh))
		hiddenChartRefresh();
}

function hiddenChartRefresh() {
	refreshHiddenChart(triggerRefresh);
}

function roundRobinRefresh(charts, startat) {
	var refreshed = false;

	// find a chart to refresh
	var all = charts.length;
	var cur = startat;
	var count = 0;

	for(count = 0; count < all ; count++, cur++) {
		if(cur >= all) cur = 0;

		if(charts[cur].enabled) {
			//mylog('going to refresh chart ' + charts[cur].name);
			refreshed = refreshChart(charts[cur], chartsRefresh);
			if(refreshed) break;
		}
	}

	if(!refreshed) triggerRefresh();
	return cur;
}

// refresh the thumb charts
// make sure we don't loose the refreshing thread
var last_thumb_updated = 0;
function thumbChartsRefresh() {
	//mylog('thumbChartsRefresh()');

	if(mycharts.length == 0 || mode != MODE_THUMBS) {
		triggerRefresh();
		return;
	}

	last_thumb_updated = roundRobinRefresh(mycharts, last_thumb_updated);
}

// refresh the group charts
// make sure we don't loose the refreshing thread
var last_group_updated = 0;
function groupChartsRefresh() {
	//mylog('groupChartsRefresh()');

	if(!group_charts || group_charts.length == 0 || mode != MODE_GROUP_THUMBS) {
		//mylog('cannot refresh charts');
		triggerRefresh();
		return;
	}

	last_group_updated = roundRobinRefresh(group_charts, last_group_updated);
}


// ------------------------------------------------------------------------
// switch the screen between views
// these should be called only from initXXXX()

function disableChart(i) {
	//mylog('disableChart(' + i + ')');

	var chart = null;

	var count = 0;
	if(mode == MODE_GROUP_THUMBS && group_charts) {
		$.each(group_charts, function(i, c) {
			if(c.enabled) count++;
		});

		if(i < group_charts.length) chart = group_charts[i];
	}
	else if(mode == MODE_THUMBS) {
		$.each(mycharts, function(i, c) {
			if(c.enabled) count++;
		});

		if(i < mycharts.length) chart = mycharts[i];
	}

	if(!chart) return;

	if(count <= 1) {
		alert('Cannot close the last chart shown.');
		return;
	}

	if(chart) {
		//mylog("disabling chart " + chart.name);
		chart.disablethisplease = true;
	}
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
	//mylog('cleanupCharts()');

	if(mode != MODE_MAIN && mainchart) {
		if(mainchart.chart) cleanThisChart(mainchart);
		mainchart = null;
	}

	if(mode != MODE_GROUP_THUMBS && group_charts) {
		clearGroupGraphs();
	}

	// cleanup the disabled charts
	$.each(mycharts, function(i, c) {
		if(c.disablethisplease && c.enabled) {
			cleanThisChart(c, 'emptydivs');
			c.disablethisplease = false;
			c.enabled = false;
			resize_request = true;
			//mylog("disabled chart " + c.name + " removed");
		}
	});

	if(group_charts) $.each(group_charts, function(i, c) {
		if(c.disablethisplease && c.enabled) {
			cleanThisChart(c, 'emptydivs');
			c.disablethisplease = false;
			c.enabled = false;
			resize_request = true;
			//mylog("disabled chart " + c.name + " removed");
		}
	});

	// we never cleanup the thumb charts
}

function updateUI() {
	$('[data-toggle="tooltip"]').tooltip({'placement': 'top', 'container': 'body', 'html': true});

	$('[data-spy="scroll"]').each(function () {
		var $spy = $(this).scrollspy('refresh')
	})
}

var thumbsScrollPosition = null;
function switchToMainGraph() {
	//mylog('switchToMainGraph()');

	if(!mainchart) return;

	if(!group_charts) thumbsScrollPosition = window.pageYOffset;

	document.getElementById('maingraph_container').style.display = 'block';
	document.getElementById('thumbgraphs_container').style.display = 'none';
	document.getElementById('groupgraphs_container').style.display = 'none';

	document.getElementById("main_menu_div").innerHTML = "<ul class=\"nav navbar-nav\"><li><a href=\"javascript:switchToThumbGraphs();\">Back to Home</a></li><li class=\"active\"><a href=\"#\">" + mainchart.title + "</a></li></ul>";

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

	document.getElementById("main_menu_div").innerHTML = mainmenu;

	if(thumbsScrollPosition) window.scrollTo(0, thumbsScrollPosition);

	// switch mode
	mode = MODE_THUMBS;
	playGraphs();
	updateUI();
}

function switchToGroupGraphs() {
	//mylog('switchToGroupGraphs()');

	if(!group_charts) return;

	if(!mainchart) thumbsScrollPosition = window.pageYOffset;

	document.getElementById('maingraph_container').style.display = 'none';
	document.getElementById('thumbgraphs_container').style.display = 'none';
	document.getElementById('groupgraphs_container').style.display = 'block';

	document.getElementById("main_menu_div").innerHTML = "<ul class=\"nav navbar-nav\"><li><a href=\"javascript:switchToThumbGraphs();\">Back to Home</a></li><li class=\"active\"><a href=\"#\">" + group_charts[0].family + " charts</a></li></ul>";

	window.scrollTo(0, 0);

	mode = MODE_GROUP_THUMBS;
	playGraphs();
	updateUI();
}


// ------------------------------------------------------------------------
// Group Charts

var group_charts = null;
function initGroupGraphs(group) {
	var count = 0;
	
	if(group_charts) clearGroupGraphs();
	group_charts = new Array();

	var groupbody = "";
	$.each(mycharts, function(i, c) {
		if(c.family == group) {
			group_charts[count] = [];
			group_charts[count] = $.extend(true, {}, c);
			group_charts[count].div += "_group";
			group_charts[count].enabled = true;
			group_charts[count].chart = null;
			group_charts[count].last_updated = 0;
			count++;
		}
	});
	group_charts.sort(chartssort);

	var sizes = groupChartSizes();

	var groupbody = "";
	$.each(group_charts, function(i, c) {
		c.chartOptions.width = sizes.width;
		c.chartOptions.height = sizes.height;

		calculateChartPointsToShow(c, c.chartOptions.isStacked?GROUPS_STACKED_POINTS_DIVISOR:GROUPS_POINTS_DIVISOR, GROUPS_MAX_TIME_TO_SHOW, -1);

		groupbody += "<div class=\"thumbgraph\" id=\"" + c.div + "_parent\"><table><tr><td><div class=\"thumbgraph\" id=\"" + c.div + "\">" + chartIsLoadingHTML(c.name, c.chartOptions.width, c.chartOptions.height) + "</div></td></tr><tr><td align=\"center\">" + thumbChartActions(i, c, 'nogroup') + "</td></tr><tr><td height='15'></td></tr></table></div>";
	});
	groupbody += "";

	document.getElementById("groupgraphs").innerHTML = groupbody;
	switchToGroupGraphs();
}

function clearGroupGraphs() {
	if(group_charts && group_charts.length) {
		$.each(group_charts, function(i, c) {
			cleanThisChart(c, 'emptydivs');
		});

		group_charts = null;
	}

	document.getElementById("groupgraphs").innerHTML = "";
}


// ------------------------------------------------------------------------
// Global entry point
// initialize the thumb charts

// load the charts from the server
// generate html for the thumbgraphs to support them
function initCharts() {
	var width = thumbWidth();
	var height = TARGET_THUMB_GRAPH_HEIGHT;

	loadCharts(null, function(all) {
		mycharts = all.charts;

		if(mycharts == null || mycharts.length == 0) {
			alert("Cannot load data from server.");
			return;
		}

		var thumbsContainer = document.getElementById("thumbgraphs");
		if(!thumbsContainer) {
			alert("Cannot find the thumbsContainer");
			return;
		}

		mycharts.sort(chartssort);

		document.getElementById('hostname_id').innerHTML = all.hostname;
		document.title = all.hostname;

		// create an array for grouping all same-type graphs together
		var dimensions = 0;
		var categories = new Array();
		$.each(mycharts, function(i, c) {
			dimensions += c.dimensions.length;
			c.chartOptions.width = width;
			c.chartOptions.height = height;

			// calculate how many point to show for each chart
			//c.points_to_show = Math.round(c.entries / c.group) - 1;
			// show max 10 mins of data
			//if(c.points_to_show * c.group > THUMBS_MAX_TIME_TO_SHOW) c.points_to_show = THUMBS_MAX_TIME_TO_SHOW / c.group;
			calculateChartPointsToShow(c, c.chartOptions.isStacked?THUMBS_STACKED_POINTS_DIVISOR:THUMBS_POINTS_DIVISOR, THUMBS_MAX_TIME_TO_SHOW, -1);

			if(c.enabled) {
				var j;
				var h = "<div class=\"thumbgraph\" id=\"" + c.div + "_parent\"><table><tr><td><div class=\"thumbgraph\" id=\"" + c.div + "\">" + chartIsLoadingHTML(c.name, c.chartOptions.width, c.chartOptions.height) + "</div></td></tr><tr><td align=\"center\">"
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

				if(j == categories.length) {
					categories.push({name: c.type, title: c.category, description: '', priority: 0, count: 1, glyphicon: c.glyphicon, html: h});
				}
			}
		});

		document.getElementById('server_summary_id').innerHTML = "<small>NetData server at <b>" + all.hostname + "</b> is maintaining <b>" + mycharts.length + "</b> charts, having <b>" + dimensions + "</b> dimensions (by default with <b>" + all.history + "</b> entries each), which are updated every <b>" + all.update_every + "s</b>, using a total of <b>" + (Math.round(all.memory * 10 / 1024 / 1024) / 10) + " MB</b> for the round robin database.</small>";

		$.each(categories, function(i, a) {
			     if(a.name == "system") a.priority = 1;
			else if(a.name == "net") a.priority = 2;
			else if(a.name == "tc") a.priority = 3;
			else if(a.name == "conntrack") a.priority = 4;
			else if(a.name == "ipvs") a.priority = 5;
			else if(a.name == "ipv4") a.priority = 6;
			else if(a.name == "cpu") a.priority = 7;
			else if(a.name == "mem") a.priority = 8;
			else if(a.name == "disk") a.priority = 9;
			else a.priority = 99;

			a.html = "<tr><td id=\"" + a.name + "\"><ol class=\"breadcrumb graphs\"><li class=\"active\"><span class=\"glyphicon " + a.glyphicon + "\"></span> &nbsp; <a id=\"" + a.name + "\" href=\"#" + a.name + "\"><b>" + a.title + "</b> " + a.description + " </a></li></ol></td></tr><tr><td><div class=\"thumbgraphs\">" + a.html + "</td></tr>";
		});

		function categoriessort(a, b) {
			if(a.priority < b.priority) return -1;
			return 1;
		}
		categories.sort(categoriessort);
		
		// combine all the htmls into one
		var allcategories = "<table width=\"100%\">";
		mainmenu = "<ul class=\"nav navbar-nav\"><li><a href=\"#\">Home</a></li>";
		$.each(categories, function(i, a) {
			allcategories += a.html;
			mainmenu += "<li><a href=\"#" + a.name + "\">" + a.title + "</a></li>";
		});
		allcategories += "</table>";
		mainmenu += "</ul>";

		thumbsContainer.innerHTML = allcategories;
		switchToThumbGraphs();
		checkRefreshThread();
	});
}

// Set a callback to run when the Google Visualization API is loaded.
google.setOnLoadCallback(initCharts);

