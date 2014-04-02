// fix old IE bug with console
if(!window.console){ window.console = {log: function(){} }; }

// Load the Visualization API and the piechart package.
google.load('visualization', '1', {'packages':['corechart']});

function refreshChart(chart, doNext) {
	chart.refreshCount++;
	
	if(chart.chart != null) {
		if(chart.chart.getSelection()[0]) {
			if(typeof doNext == "function") doNext();
			return;
		}
	}
	
	var url = chart.url;
	url += chart.points_to_show?chart.points_to_show.toString():"all";
	url += "/";
	url += chart.group?chart.group.toString():"1";
	url += "/";
	url += chart.group_method?chart.group_method:"average";

	$.ajax({
		url: url,
		dataType:"json",
		cache: false
	}).done(function(jsondata) {
		if(!jsondata || jsondata.length == 0) return;
		chart.jsondata = jsondata;
		
		// Create our data table out of JSON data loaded from server.
		chart.datatable = new google.visualization.DataTable(chart.jsondata);
		
		// cleanup once every 50 updates
		// we don't cleanup on every single, to avoid firefox flashing effect
		if(chart.chart && chart.refreshCount > 50) {
			chart.chart.clearChart();
			chart.chart = null;
		}

		// Instantiate and draw our chart, passing in some options.
		if(!chart.chart) {
			console.log('Creating new chart for ' + chart.url);
			if(chart.chartType == "LineChart")
				chart.chart = new google.visualization.LineChart(document.getElementById(chart.div));
			else
				chart.chart = new google.visualization.AreaChart(document.getElementById(chart.div));
		}
		
		if(chart.chart) chart.chart.draw(chart.datatable, chart.chartOptions);
		else console.log('Cannot create chart for ' + chart.url);

		if(typeof doNext == "function") doNext();
	});
}

// loadCharts()
// fetches all the charts from the server
// returns an array of objects, containing all the server metadata
// (not the values of the graphs - just the info about the graphs)

function loadCharts(doNext) {
	$.ajax({
		url: '/all.json',
		dataType: 'json',
		cache: false
	}).done(function(json) {
		$.each(json.charts, function(i, value) {
			json.charts[i].div = json.charts[i].name.replace(/\./g,"_");
			json.charts[i].div = json.charts[i].div.replace(/\-/g,"_");
			json.charts[i].div = json.charts[i].div + "_div";

			json.charts[i].thumbnail = false;
			json.charts[i].refreshCount = 0;
			json.charts[i].group = 1;
			json.charts[i].points_to_show = 0;	// all
			json.charts[i].group_method = "max";

			json.charts[i].chart = null;
			json.charts[i].jsondata = null;
			json.charts[i].datatable = null;
			
			// if the user has given a title, use it
			if(json.charts[i].usertitle) json.charts[i].title = json.charts[i].usertitle;

			// check if the userpriority is IGNORE
			if(json.charts[i].userpriority == "IGNORE")
				json.charts[i].enabled = false;
			else
				json.charts[i].enabled = true;

			// set default chart options
			json.charts[i].chartOptions = {
				lineWidth: 2,
				title: json.charts[i].title,
				hAxis: {title: "Time of Day", viewWindowMode: 'maximized', format:'HH:mm:ss'},
				vAxis: {title: json.charts[i].vtitle, minValue: 0},
				focusTarget: 'category',
			};

			// set the chart type
			if((json.charts[i].type == "ipv4"
				|| json.charts[i].type == "ipv6"
				|| json.charts[i].type == "ipvs"
				|| json.charts[i].type == "conntrack"
				|| json.charts[i].id == "cpu.ctxt"
				|| json.charts[i].id == "cpu.intr"
				|| json.charts[i].id == "cpu.processes"
				|| json.charts[i].id == "cpu.procs_running"
				)
				&& json.charts[i].name != "ipv4.net" && json.charts[i].name != "ipvs.net") {
				
				// default for all LineChart
				json.charts[i].chartType = "LineChart";
				json.charts[i].chartOptions.lineWidth = 3;
				json.charts[i].chartOptions.curveType = 'function';
			}
			else if(json.charts[i].type == "tc" || json.charts[i].type == "cpu") {

				// default for all stacked AreaChart
				json.charts[i].chartType = "AreaChart";
				json.charts[i].chartOptions.isStacked = true;
				json.charts[i].chartOptions.areaOpacity = 1.0;
				json.charts[i].chartOptions.lineWidth = 1;

				json.charts[i].group_method = "average";
			}
			else {

				// default for all AreaChart
				json.charts[i].chartType = "AreaChart";
				json.charts[i].chartOptions.isStacked = false;
				json.charts[i].chartOptions.areaOpacity = 0.3;
			}

			// the category name, and other options, per type
			switch(json.charts[i].type) {
				case "cpu":
					json.charts[i].category = "CPU";
					json.charts[i].glyphicon = "glyphicon-dashboard";
					json.charts[i].group = 15;

					if(json.charts[i].id.substring(0, 7) == "cpu.cpu") {
						json.charts[i].chartOptions.vAxis.minValue = 0;
						json.charts[i].chartOptions.vAxis.maxValue = 100;
					}
					break;

				case "tc":
					json.charts[i].category = "Quality of Service";
					json.charts[i].glyphicon = "glyphicon-random";
					json.charts[i].group = 30;
					break;

				case "net":
					json.charts[i].category = "Network Interfaces";
					json.charts[i].glyphicon = "glyphicon-transfer";
					json.charts[i].group = 10;

					// disable IFB and net.lo devices by default
					if((json.charts[i].id.substring(json.charts[i].id.length - 4, json.charts[i].id.length) == "-ifb")
						|| json.charts[i].id == "net.lo")
						json.charts[i].enabled = false;
					break;

				case "ipv4":
					json.charts[i].category = "IPv4";
					json.charts[i].glyphicon = "glyphicon-globe";
					json.charts[i].group = 20;
					break;

				case "conntrack":
					json.charts[i].category = "Netfilter";
					json.charts[i].glyphicon = "glyphicon-cloud";
					json.charts[i].group = 20;
					break;

				case "ipvs":
					json.charts[i].category = "IPVS";
					json.charts[i].glyphicon = "glyphicon-sort";
					json.charts[i].group = 15;
					break;

				case "disk":
					json.charts[i].category = "Disk I/O";
					json.charts[i].glyphicon = "glyphicon-hdd";
					json.charts[i].group = 15;
					break;

				default:
					json.charts[i].category = "Unknown";
					json.charts[i].glyphicon = "glyphicon-search";
					json.charts[i].group = 30;
					break;
			}
		});

		if(typeof doNext == "function") doNext(json.charts);
	});
};

var charts = new Array();
function addChart(name, div, width, height, jsonurl, title, vtitle) {
	var i = charts.length;
	
	console.log('Creating new objects for chart ' + name);
	charts[i] = [];

	charts[i].name = name;
	charts[i].title = title;
	charts[i].vtitle = vtitle;
	charts[i].url = jsonurl;

	charts[i].div = div;

	charts[i].refreshCount = 0;
	charts[i].thumbnail = false;

	charts[i].chart = null;
	charts[i].jsondata = null;
	charts[i].datatable = null;

	charts[i].chartType = "AreaChart";
	charts[i].chartOptions = {
		width: width,
		height: height,
		title: charts[i].title,
		hAxis: {title: "Time of Day"},
		vAxis: {title: charts[i].vtitle, minValue: 10},
		focusTarget: 'category',
	};
}

var charts_last_drawn = 999999999;
function refreshCharts(howmany) {
	
	if(charts.length == 0) return;
	
	var h = howmany;
	if(h == 0) h = charts.length;
	if(h > charts.length) h = charts.length;
	//console.log('Will run for ' + h + ' charts');
	
	var width = Math.round(Math.sqrt(charts.length));
	var height = Math.round(Math.sqrt(charts.length));
	while((width * height) < charts.length) {
		if((height + 1) <= width) height++;
		else width++;
	}
	// console.log('all: ' + charts.length + ', optimal: width = ' + width + ', height = ' + height);

	while(width > 1 && width >= height) {
		width--;
		height++;
	}
	if(width * height < charts.length) height++;
	// console.log('final: width = ' + width + ', height = ' + height);

	ww = (window.innerWidth < document.documentElement.clientWidth)?window.innerWidth:document.documentElement.clientWidth;
	wh = (window.innerHeight < document.documentElement.clientHeight)?window.innerHeight:document.documentElement.clientHeight;

	if(width == 0) width = (ww - 40) / 2;
	if(width <= 10) width = (ww - 40) / width;
	if(width < 200) width = 200;
	
	if(height == 0) height = (wh - 20) / 2;
	if(height <= 10) height = (wh - 20) / height;
	if(height < 100) height = 100;

	// console.log('width = ' + width + ', height = ' + height);

	var i;
	for(i = 0; i < h; i++) {
		var zeroDimensions = 0;

		charts_last_drawn++;
		if(charts_last_drawn >= charts.length) charts_last_drawn = 0;
		
		if(charts[charts_last_drawn].chartOptions.width == 0 && charts[charts_last_drawn].height == 0) {
			charts[charts_last_drawn].chartOptions.width = width;
			charts[charts_last_drawn].chartOptions.height = height;
			zeroDimensions = 1;
		}

		try {

			console.log('Refreshing chart ' + charts[charts_last_drawn].name);
			refreshChart(charts[charts_last_drawn]);
		}
		catch(err) {
			console.log('Cannot refresh chart for ' + charts[charts_last_drawn].url);
		}

		if(zeroDimensions == 1) {
			charts[charts_last_drawn].chartOptions.width = 0;
			charts[charts_last_drawn].chartOptions.height = 0;
		}
	}
	return 0;
}
