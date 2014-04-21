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
	var now = new Date().getTime();
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

	return url;
}

function refreshChart(chart, doNext) {
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

/*
function showChartWithRange(chart) {
	var oldgroup = chart.group;
	var oldpoints = chart.points_to_show;
	chart.group = 1;
	chart.points_to_show = 0; // all

	$.ajax({
		url: generateChartURL(chart),
		dataType:"json",
		cache: false
	})
	.done(function(jsondata) {
		chart.jsondata = jsondata;

		chart.control_div = chart.div + "_control";
		chart.chart_div = chart.div + "_chart";

		var div = document.getElementById(chart.div);
		div.innerHTML = "<div class=\"maingraph\" id=\"" + chart.chart_div + "\"></div><div class=\"maingraph\" id=\"" + chart.control_div + "\"></div>";
		//div.width = chart.chartOptions.width;
		//div.height = chart.chartOptions.height;

		console.log(chart.div);
		chart.dashboard = new google.visualization.Dashboard(div);
		console.log('dashboard ok');
		console.log(chart.dashboard);

		chart.data = new google.visualization.DataTable(chart.jsondata);
		var range = chart.data.getColumnRange(0);

		chart.control = new google.visualization.ControlWrapper({
			'controlType': 'ChartRangeFilter',
			'containerId': chart.control_div,
			'options': {
				// Filter by the date axis.
				'filterColumnIndex': 0,
				'ui': {
					'chartType': 'AreaChart',
					'chartOptions': {
						'height': chart.chartOptions.height * 0.15,
						'width': chart.chartOptions.width,
						'chartArea': {'width': "90%"},
						'hAxis': {'baselineColor': 'none'}
					},
					
					//'chartView': {
					//	'columns': [0, 1]
					//},

					// 1 day in milliseconds = 24 * 60 * 60 * 1000 = 86,400,000
					//'minRangeSize': 86400000
				}
			},

			// Initial range: 2012-02-09 to 2012-03-20.
			'state': {'range': {'start': range.min, 'end': range.max}}
		});
		console.log('control ok');
		console.log(chart.control);

		var columns = new Array();
		var i;
		for (i = 0; i < chart.data.getNumberOfColumns(); i++)
			columns[i] = i;

		columns[0] = {
			'calc': function(dataTable, rowIndex) {
				return dataTable.getFormattedValue(rowIndex, 0);
			},
			'type': 'string'
		};

		
		chart.chartOptions.height = chart.chartOptions.height * 0.85;
		chart.chartOptions.chartArea = {'height': "80%", 'width': "90%"};
		chart.chartOptions.legend = 'none';

		chart.chartwrap = new google.visualization.ChartWrapper({
			'chartType': chart.chartType,
			'containerId': chart.chart_div,
			'options': chart.chartOptions,
		});
		console.log('chart ok');
		console.log(chart.chartwrap);

		chart.dashboard.bind(chart.control, chart.chartwrap);
		console.log('bind ok');

		chart.dashboard.draw(chart.data);
		console.log('draw ok');

		// restore what we changed
		chart.group = oldgroup;
		chart.points_to_show = oldpoints;
	});
}
*/

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

			// if the user has given a title, use it
			if(json.charts[i].usertitle) json.charts[i].title = json.charts[i].usertitle;

			// check if the userpriority is IGNORE
			if(json.charts[i].userpriority == "IGNORE"
				|| json.charts[i].isdetail
				)
				json.charts[i].enabled = false;
			else
				json.charts[i].enabled = true;

			// set default chart options
			json.charts[i].chartOptions = {
				width: 400,
				height: 200,
				lineWidth: 1,
				title: json.charts[i].title,
				hAxis: {title: "Time of Day", viewWindowMode: 'maximized', format:'HH:mm:ss'},
				vAxis: {title: json.charts[i].units, viewWindowMode: 'pretty', minValue: 0, maxValue: 10},
				focusTarget: 'category',
				annotation: {'1': {style: 'line'}},
				//colors: ['blue', 'red', 'green', 'lime', 'olive', 'yellow', 'navy', 'fuchsia', 'maroon', 'aqua', 'teal', 'purple', 'black', 'gray', 'silver'],
				//tooltip: {isHtml: true},
			};

			// set the chart type
			if(json.charts[i].type == "tc"
				|| json.charts[i].id.substring(0, 7) == "cpu.cpu"
				|| json.charts[i].name == 'system.cpu'
				|| json.charts[i].name == 'system.ram'
				|| json.charts[i].name == 'system.swap'
				|| json.charts[i].name == 'mem.slab'
				|| json.charts[i].name == 'mem.kernel'
				) {

				// default for all stacked AreaChart
				json.charts[i].chartType = "AreaChart";
				json.charts[i].chartOptions.isStacked = true;
				json.charts[i].chartOptions.areaOpacity = 0.85;
				json.charts[i].chartOptions.lineWidth = 1;
				json.charts[i].chartOptions.vAxis.viewWindowMode = 'maximized';

				json.charts[i].group_method = "average";
			}
			else if(json.charts[i].type == "net"
				|| json.charts[i].type == "disk"
				|| json.charts[i].id == "system.ipv4"
				|| json.charts[i].id == "system.io"
				|| json.charts[i].id == "system.swapio"
				|| json.charts[i].id == "ipv4.mcast"
				|| json.charts[i].id == "ipv4.bcast"
				|| json.charts[i].id == "mem.committed"
				) {

				// default for all AreaChart
				json.charts[i].chartType = "AreaChart";
				json.charts[i].chartOptions.isStacked = false;
				json.charts[i].chartOptions.areaOpacity = 0.3;
			}
			else {
				
				// default for all LineChart
				json.charts[i].chartType = "LineChart";
				json.charts[i].chartOptions.lineWidth = 2;
				json.charts[i].chartOptions.curveType = 'function';

				json.charts[i].chartOptions.vAxis.minValue = -10;
				json.charts[i].chartOptions.vAxis.maxValue =  10;
			}

			// the category name, and other options, per type
			switch(json.charts[i].type) {
				case "system":
					json.charts[i].category = "System";
					json.charts[i].glyphicon = "glyphicon-dashboard";
					json.charts[i].group = 5;

					if(json.charts[i].id == "system.cpu" || json.charts[i].id == "system.ram") {
						json.charts[i].chartOptions.vAxis.minValue = 0;
						json.charts[i].chartOptions.vAxis.maxValue = 100;
					}
					else {
						json.charts[i].chartOptions.vAxis.minValue = -10;
						json.charts[i].chartOptions.vAxis.maxValue =  10;
					}
					break;

				case "cpu":
					json.charts[i].category = "CPU";
					json.charts[i].glyphicon = "glyphicon-dashboard";
					json.charts[i].group = 5;

					if(json.charts[i].id.substring(0, 7) == "cpu.cpu") {
						json.charts[i].chartOptions.vAxis.minValue = 0;
						json.charts[i].chartOptions.vAxis.maxValue = 100;
					}
					break;

				case "mem":
					json.charts[i].category = "Memory";
					json.charts[i].glyphicon = "glyphicon-dashboard";
					json.charts[i].group = 5;
					break;

				case "tc":
					json.charts[i].category = "QoS";
					json.charts[i].glyphicon = "glyphicon-random";
					json.charts[i].group = 15;
					break;

				case "net":
					json.charts[i].category = "Network";
					json.charts[i].glyphicon = "glyphicon-transfer";
					json.charts[i].group = 5;

					// disable IFB and net.lo devices by default
					if((json.charts[i].id.substring(json.charts[i].id.length - 4, json.charts[i].id.length) == "-ifb")
						|| json.charts[i].id == "net.lo")
						json.charts[i].enabled = false;
					break;

				case "ipv4":
					json.charts[i].category = "IPv4";
					json.charts[i].glyphicon = "glyphicon-globe";
					json.charts[i].group = 5;
					break;

				case "conntrack":
					json.charts[i].category = "Netfilter";
					json.charts[i].glyphicon = "glyphicon-cloud";
					json.charts[i].group = 5;
					break;

				case "ipvs":
					json.charts[i].category = "IPVS";
					json.charts[i].glyphicon = "glyphicon-sort";
					json.charts[i].group = 5;
					break;

				case "disk":
					json.charts[i].category = "Disks";
					json.charts[i].glyphicon = "glyphicon-hdd";
					json.charts[i].group = 5;
					break;

				default:
					json.charts[i].category = json.charts[i].type;
					json.charts[i].glyphicon = "glyphicon-search";
					json.charts[i].group = 5;
					break;
			}
		});
		
		if(typeof doNext == "function") doNext(json.charts);
	})
	.fail(function() {
		if(typeof doNext == "function") doNext();
	});
};
