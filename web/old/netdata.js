// fix old IE bug with console
if(!window.console){ window.console = {log: function(){} }; }

// Load the Visualization API and the piechart package.
google.load('visualization', '1.1', {'packages':['corechart']});
//google.load('visualization', '1.1', {'packages':['controls']});

function canChartBeRefreshed(chart) {
	// is it enabled?
	if(!chart.enabled) return false;

	// is there something selected on the chart?
	if(chart.chart && chart.chart.getSelection()[0]) return false;

	// is it too soon for a refresh?
	var now = Date.now();
	if((now - chart.last_updated) < (chart.group * chart.update_every * 1000)) return false;

	// is the chart in the visible area?
	//console.log(chart.div);
	if($('#' + chart.div).visible(true) == false) return false;

	// ok, do it
	return true;
}

function generateChartURL(chart) {
	// build the data URL
	var url = chart.url;
	url += chart.points_to_show?chart.points_to_show.toString():"all";
	url += "/";
	url += chart.group?chart.group.toString():"1";
	url += "/";
	url += chart.group_method?chart.group_method:"average";
	url += "/";
	url += chart.after?chart.after.toString():"0";
	url += "/";
	url += chart.before?chart.before.toString():"0";
	url += "/";
	url += chart.non_zero?"nonzero":"all";
	url += "/";

	return url;
}

function renderChart(chart, doNext) {
	if(canChartBeRefreshed(chart) == false) return false;

	$.ajax({
		url: generateChartURL(chart),
		dataType:"json",
		cache: false
	})
	.done(function(jsondata) {
		if(!jsondata || jsondata.length == 0) return;
		chart.jsondata = jsondata;

		// Create our data table out of JSON data loaded from server.
		chart.datatable = new google.visualization.DataTable(chart.jsondata);
		//console.log(chart.datatable);

		// cleanup once every 50 updates
		// we don't cleanup on every single, to avoid firefox flashing effect
		if(chart.chart && chart.refreshCount > 50) {
			chart.chart.clearChart();
			chart.chart = null;
			chart.refreshCount = 0;
		}

		// Instantiate and draw our chart, passing in some options.
		if(!chart.chart) {
			// console.log('Creating new chart for ' + chart.url);
			if(chart.chartType == "LineChart")
				chart.chart = new google.visualization.LineChart(document.getElementById(chart.div));
			else
				chart.chart = new google.visualization.AreaChart(document.getElementById(chart.div));
		}

		if(chart.chart) {
			chart.chart.draw(chart.datatable, chart.chartOptions);
			chart.refreshCount++;
			chart.last_updated = Date.now();
		}
		else console.log('Cannot create chart for ' + chart.url);
	})
	.fail(function() {
		// to avoid an infinite loop, let's assume it was refreshed
		if(chart.chart) chart.chart.clearChart();
		chart.chart = null;
		chart.refreshCount = 0;
		showChartIsLoading(chart.div, chart.name, chart.chartOptions.width, chart.chartOptions.height, "failed to refresh");
		chart.last_updated = Date.now();
	})
	.always(function() {
		if(typeof doNext == "function") doNext();
	});

	return true;
}

function chartIsLoadingHTML(name, width, height, message)
{
	return "<table><tr><td align=\"center\" width=\"" + width + "\" height=\"" + height + "\" style=\"vertical-align:middle\"><h4><span class=\"glyphicon glyphicon-refresh\"></span><br/><br/>" + name + "<br/><br/><span class=\"label label-default\">" + (message?message:"loading chart...") + "</span></h4></td></tr></table>";
}

function showChartIsLoading(id, name, width, height, message) {
	document.getElementById(id).innerHTML = chartIsLoadingHTML(name, width, height, message);
}

// calculateChartPointsToShow
// calculate the chart group and point to show properties.
// This uses the chartOptions.width and the supplied divisor
// to calculate the propers values so that the chart will
// be visually correct (not too much or too less points shown).
//
// c = the chart
// divisor = when calculating screen points, divide width with this
//           if all screen points are used the chart will be overcrowded
//           the default is 2
// maxtime = the maxtime to show
//           the default is to render all the server data
// group   = the required grouping on points
//           if undefined or negative, any calculated value will be used
//           if zero, one of 1,2,5,10,15,20,30,45,60 will be used

function calculateChartPointsToShow(c, divisor, maxtime, group, enable_curve) {
	// console.log('calculateChartPointsToShow( c = ' + c.id + ',  divisor = ' + divisor + ', maxtime = ' + maxtime + ', group = ' + group + ' )');

	if(!divisor) divisor = 2;

	var before = c.before?c.before:Date.now() / 1000;
	var after = c.after?c.after:c.first_entry_t;

	var dt = before - after;
	if(dt > c.entries * c.update_every) dt = c.entries * c.update_every;

	// console.log('chart ' + c.id + ' internal duration is ' + dt + ' secs, requested maxtime is ' + maxtime + ' secs');

	if(!maxtime) maxtime = c.entries * c.update_every;
	dt = maxtime;

	var data_points = Math.round(dt / c.update_every);
	if(!data_points) data_points = 100;

	var screen_points = Math.round(c.chartOptions.width / divisor);
	if(!screen_points) screen_points = 100;

	// console.log('screen_points = ' + screen_points + ', data_points = ' + data_points + ', divisor = ' + divisor);

	if(group == undefined || group <= 0) {
		if(screen_points > data_points) {
			c.group = 1;
			c.points_to_show = data_points;
			// console.log("rendering at full detail (group = " + c.group + ", points_to_show = " + c.points_to_show + ')');
		}
		else {
			c.group = Math.round(data_points / screen_points);

			     if(c.group > 60) c.group = 90;
			else if(c.group > 45) c.group = 60;
			else if(c.group > 30) c.group = 45;
			else if(c.group > 20) c.group = 30;
			else if(c.group > 15) c.group = 20;
			else if(c.group > 10) c.group = 15;
			else if(c.group > 5) c.group = 10;
			else if(c.group > 4) c.group = 5;
			else if(c.group > 3) c.group = 4;
			else if(c.group > 2) c.group = 3;
			else if(c.group > 1) c.group = 2;
			else c.group = 1;

			c.points_to_show = Math.round(data_points / c.group);
			// console.log("rendering adaptive (group = " + c.group + ", points_to_show = " + c.points_to_show + ')');
		}
	}
	else {
		c.group = group;
		c.points_to_show = Math.round(data_points / group);
		// console.log("rendering with supplied group (group = " + c.group + ", points_to_show = " + c.points_to_show + ')');
	}

	// console.log("final configuration (group = " + c.group + ", points_to_show = " + c.points_to_show + ')');

	// make sure the line width is not congesting the chart
	if(c.chartType == 'LineChart') {
		if(c.points_to_show > c.chartOptions.width / 3) {
			c.chartOptions.lineWidth = 1;
		}

		else {
			c.chartOptions.lineWidth = 2;
		}
	}
	else if(c.chartType == 'AreaChart') {
		if(c.points_to_show > c.chartOptions.width / 2)
			c.chartOptions.lineWidth = 0;
		else
			c.chartOptions.lineWidth = 1;
	}

	// do not render curves when we don't have at
	// least 2 twice the space per point
	if(!enable_curve || c.points_to_show > (c.chartOptions.width * c.chartOptions.lineWidth / 2) )
		c.chartOptions.curveType = 'none';
	else
		c.chartOptions.curveType = c.default_curveType;

	var hpoints = Math.round(maxtime / 30);
	if(hpoints > 10) hpoints = 10;
	c.chartOptions.hAxis.gridlines.count = hpoints;
}


// loadCharts()
// fetches all the charts from the server
// returns an array of objects, containing all the server metadata
// (not the values of the graphs - just the info about the graphs)

function loadCharts(base_url, doNext) {
	$.ajax({
		url: ((base_url)?base_url:'') + '/all.json',
		dataType: 'json',
		cache: false
	})
	.done(function(json) {
		$.each(json.charts, function(i, value) {
			json.charts[i].div = json.charts[i].name.replace(/\./g,"_");
			json.charts[i].div = json.charts[i].div.replace(/\-/g,"_");
			json.charts[i].div = json.charts[i].div + "_div";

			// make sure we have the proper values
			if(!json.charts[i].update_every) chart.update_every = 1;
			if(base_url) json.charts[i].url = base_url + json.charts[i].url;

			json.charts[i].last_updated = 0;
			json.charts[i].thumbnail = false;
			json.charts[i].refreshCount = 0;
			json.charts[i].group = 1;
			json.charts[i].points_to_show = 0;	// all
			json.charts[i].group_method = "max";

			json.charts[i].chart = null;
			json.charts[i].jsondata = null;
			json.charts[i].datatable = null;
			json.charts[i].before = 0;
			json.charts[i].after = 0;

			// if it is detail, disable it by default
			if(json.charts[i].isdetail) json.charts[i].enabled = false;

			// set default chart options
			json.charts[i].chartOptions = {
				width: 400,
				height: 200,
				lineWidth: 1,
				title: json.charts[i].title,
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
					title: json.charts[i].units,
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
					isHtml: true,
					ignoreBounds: true,
					textStyle: {
						fontSize: 9
					}
				}
			};

			json.charts[i].default_curveType = 'none';

			// set the chart type
			switch(json.charts[i].chart_type) {
				case "area":
					json.charts[i].chartType = "AreaChart";
					json.charts[i].chartOptions.isStacked = false;
					json.charts[i].chartOptions.areaOpacity = 0.3;

					json.charts[i].chartOptions.vAxis.viewWindowMode = 'maximized';
					json.charts[i].non_zero = 0;

					json.charts[i].group = 3;
					break;

				case "stacked":
					json.charts[i].chartType = "AreaChart";
					json.charts[i].chartOptions.isStacked = true;
					json.charts[i].chartOptions.areaOpacity = 0.85;
					json.charts[i].chartOptions.lineWidth = 1;
					json.charts[i].group_method = "average";
					json.charts[i].non_zero = 1;

					json.charts[i].chartOptions.vAxis.viewWindowMode = 'maximized';
					json.charts[i].chartOptions.vAxis.minValue = null;
					json.charts[i].chartOptions.vAxis.maxValue = null;

					json.charts[i].group = 10;
					break;

				default:
				case "line":
					json.charts[i].chartType = "LineChart";
					json.charts[i].chartOptions.lineWidth = 2;
					json.charts[i].non_zero = 0;

					json.charts[i].default_curveType = 'function';

					json.charts[i].group = 3;
					break;
			}

			// the category name, and other options, per type
			switch(json.charts[i].type) {
				case "system":
					json.charts[i].category = "System";
					json.charts[i].categoryPriority = 10;
					json.charts[i].glyphicon = "glyphicon-dashboard";

					if(json.charts[i].id == "system.cpu" || json.charts[i].id == "system.ram") {
						json.charts[i].chartOptions.vAxis.minValue = 0;
						json.charts[i].chartOptions.vAxis.maxValue = 100;
					}
					else {
						json.charts[i].chartOptions.vAxis.minValue = -0.1;
						json.charts[i].chartOptions.vAxis.maxValue =  0.1;
					}
					break;

				case "net":
					json.charts[i].category = "Network";
					json.charts[i].categoryPriority = 20;
					json.charts[i].glyphicon = "glyphicon-transfer";

					// disable IFB and net.lo devices by default
					if((json.charts[i].id.substring(json.charts[i].id.length - 4, json.charts[i].id.length) == "-ifb")
						|| json.charts[i].id == "net.lo")
						json.charts[i].enabled = false;
					break;

				case "tc":
					json.charts[i].category = "Quality of Service";
					json.charts[i].categoryPriority = 30;
					json.charts[i].glyphicon = "glyphicon-random";
					break;

				case "ipvs":
					json.charts[i].category = "IP Virtual Server";
					json.charts[i].categoryPriority = 40;
					json.charts[i].glyphicon = "glyphicon-sort";
					break;

				case "netfilter":
					json.charts[i].category = "Netfilter";
					json.charts[i].categoryPriority = 50;
					json.charts[i].glyphicon = "glyphicon-cloud";
					break;

				case "ipv4":
					json.charts[i].category = "IPv4";
					json.charts[i].categoryPriority = 60;
					json.charts[i].glyphicon = "glyphicon-globe";
					break;

				case "mem":
					json.charts[i].category = "Memory";
					json.charts[i].categoryPriority = 70;
					json.charts[i].glyphicon = "glyphicon-dashboard";
					break;

				case "cpu":
					json.charts[i].category = "CPU";
					json.charts[i].categoryPriority = 80;
					json.charts[i].glyphicon = "glyphicon-dashboard";

					if(json.charts[i].id.substring(0, 7) == "cpu.cpu") {
						json.charts[i].chartOptions.vAxis.minValue = 0;
						json.charts[i].chartOptions.vAxis.maxValue = 100;
					}
					break;

				case "disk":
					json.charts[i].category = "Disks";
					json.charts[i].categoryPriority = 90;
					json.charts[i].glyphicon = "glyphicon-hdd";
					break;

				case "nfsd":
					json.charts[i].category = "NFS Server";
					json.charts[i].categoryPriority = 100;
					json.charts[i].glyphicon = "glyphicon-hdd";
					break;

				case "nut":
					json.charts[i].category = "UPS";
					json.charts[i].categoryPriority = 110;
					json.charts[i].glyphicon = "glyphicon-dashboard";
					break;

				case "netdata":
					json.charts[i].category = "NetData";
					json.charts[i].categoryPriority = 3000;
					json.charts[i].glyphicon = "glyphicon-thumbs-up";
					break;

				case "apps":
					json.charts[i].category = "Apps";
					json.charts[i].categoryPriority = 4000;
					json.charts[i].glyphicon = "glyphicon-tasks";
					break;

				case "squid":
					json.charts[i].category = "Squid";
					json.charts[i].categoryPriority = 5000;
					json.charts[i].glyphicon = "glyphicon-link";
					break;

				case "example":
					json.charts[i].category = "Examples";
					json.charts[i].categoryPriority = 9000;
					json.charts[i].glyphicon = "glyphicon-search";
					break;

				default:
					json.charts[i].category = json.charts[i].type;
					json.charts[i].categoryPriority = 1000;
					json.charts[i].glyphicon = "glyphicon-search";
					break;
			}
		});

		if(typeof doNext == "function") doNext(json);
	})
	.fail(function() {
		if(typeof doNext == "function") doNext();
	});
};

// jquery visible plugin
(function($){

	/**
	 * Copyright 2012, Digital Fusion
	 * Licensed under the MIT license.
	 * http://teamdf.com/jquery-plugins/license/
	 *
	 * @author Sam Sehnert
	 * @desc A small plugin that checks whether elements are within
	 *		 the user visible viewport of a web browser.
	 *		 only accounts for vertical position, not horizontal.
	 */
	$.fn.visible = function(partial){

	    var $t				= $(this),
	    	$w				= $(window),
	    	viewTop			= $w.scrollTop(),
	    	viewBottom		= viewTop + $w.height(),
	    	_top			= $t.offset().top,
	    	_bottom			= _top + $t.height(),
	    	compareTop		= partial === true ? _bottom : _top,
	    	compareBottom	= partial === true ? _top : _bottom;

		return ((compareBottom <= viewBottom) && (compareTop >= viewTop));
    };
})(jQuery);
