// fix old IE bug with console
if(!window.console){ window.console = {log: function(){} }; }

// Load the Visualization API and the piechart package.
google.load('visualization', '1', {'packages':['corechart']});

function canChartBeRefreshed(chart) {
	// is it enabled?
	if(!chart.enabled) return false;

	// is there something selected on the chart?
	if(chart.chart && chart.chart.getSelection()[0]) return false;

	// is it too soon for a refresh?
	var now = new Date().getTime();
	if((now - chart.last_updated) < (chart.group * chart.update_every * 1000)) return false;

	// is the chart in the visible area?
	//console.log(chart.div);
	if($('#' + chart.div).visible(true) == false) return false;

	// ok, do it
	return true;
}

function refreshChart(chart, doNext) {
	if(canChartBeRefreshed(chart) == false) return false;

	// build the data URL
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
	})
	.done(function(jsondata) {
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
		
		if(chart.chart) {
			chart.chart.draw(chart.datatable, chart.chartOptions);
			chart.refreshCount++;
			chart.last_updated = new Date().getTime();
		}
		else console.log('Cannot create chart for ' + chart.url);
	})
	.always(function() {
		if(typeof doNext == "function") doNext();
	});

	return true;
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
			
			// if the user has given a title, use it
			if(json.charts[i].usertitle) json.charts[i].title = json.charts[i].usertitle;

			// check if the userpriority is IGNORE
			if(json.charts[i].userpriority == "IGNORE")
				json.charts[i].enabled = false;
			else
				json.charts[i].enabled = true;

			// set default chart options
			json.charts[i].chartOptions = {
				width: 400,
				height: 200,
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
				|| json.charts[i].id == "cpu.jitter"
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
	})
	.fail(function() {
		if(typeof doNext == "function") doNext();
	});
};
