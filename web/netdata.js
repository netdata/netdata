// fix old IE bug with console
if(!window.console){ window.console = {log: function(){} }; }

// Load the Visualization API and the piechart package.
google.load('visualization', '1', {'packages':['corechart']});

function refreshChart(chart) {
	chart.refreshCount++;
	
	if(chart.chart != null) {
		if(chart.chart.getSelection()[0]) return;
	}
	
	var url = chart.url;
	url += chart.points_to_show?chart.points_to_show.toString():"all";
	url += "/";
	url += chart.group?chart.group.toString():"1";
	url += "/";
	url += chart.group_method?chart.group_method:"average";
	chart.jsondata = $.ajax({
		url: url,
		dataType:"json",
		async: false,
		cache: false
	}).responseText;
	
	if(!chart.jsondata || chart.jsondata.length == 0) return;
	
	// Create our data table out of JSON data loaded from server.
	chart.datatable = new google.visualization.DataTable(chart.jsondata);
	
	// setup the default options
	var options = {
		width: chart.width,
		height: chart.height,
		title: chart.title,
		hAxis: {title: "Time of Day"},
		vAxis: {title: chart.vtitle, minValue: 10},
		focusTarget: 'category',
		explorer: (chart.explorer)?true:false,
		// animation: {duration: 1000, easing: 'inAndOut'},
	};

	// disable interactivity on thumbnails
	if(chart.thumbnail) options.enableInteractivity = false;

	// hide axis titles if too small
	if(chart.height < 200) options.vAxis.title = null;
	if(chart.width < 350) options.hAxis.title = null;

	if(chart.name.substring(0, 5) == "ipv4." || chart.name.substring(0, 10) == "conntrack." || chart.name.substring(0, 5) == "ipvs.") {
		options.lineWidth = 3;
		options.curveType = 'function';
	}
	else if(chart.name.substring(0, 3) == "tc.") {
		options.isStacked = true;
		options.title += " [stacked]";
		options.areaOpacity = 1.0;
		options.lineWidth = 1;
	}
	else {
		options.areaOpacity = 0.3;
		options.lineWidth = 2;
	}

	// cleanup once every 100 updates
	// we don't cleanup on every single, to avoid firefox flashing effect
	if(chart.chart && chart.refreshCount > 100) {
		chart.chart.clearChart();
		chart.chart = null;
	}

	// Instantiate and draw our chart, passing in some options.
	if(!chart.chart) {
		console.log('Creating new chart for ' + chart.url);
		if(chart.name.substring(0, 5) == "ipv4." || chart.name.substring(0, 10) == "conntrack." || chart.name.substring(0, 5) == "ipvs.")
			chart.chart = new google.visualization.LineChart(document.getElementById(chart.div));
		else
			chart.chart = new google.visualization.AreaChart(document.getElementById(chart.div));
	}
	
	if(chart.chart) chart.chart.draw(chart.datatable, options);
	else console.log('Cannot create chart for ' + chart.url);
}

// loadCharts()
// fetches all the charts from the server
// returns an array of objects, containing all the server metadata
// (not the values of the graphs - just the info about the graphs)

function loadCharts() {
	var mycharts = new Array();

	$.ajax({
		url: '/all.json',
		async: false,
		dataType: 'json',
		success: function (json) { mycharts = json.charts; }
	});

	$.each(mycharts, function(i, value) {
		mycharts[i].div = mycharts[i].name.replace(/\./g,"_");
		mycharts[i].div = mycharts[i].div.replace(/\-/g,"_");
		mycharts[i].div = mycharts[i].div + "_div";

		mycharts[i].width = 0;
		mycharts[i].height = 0;
		mycharts[i].thumbnail = false;
		mycharts[i].refreshCount = 0;
		mycharts[i].group = 1;
		mycharts[i].points_to_show = 0;	// all
		mycharts[i].group_method = "average";

		mycharts[i].chart = null;
		mycharts[i].jsondata = null;
		mycharts[i].datatable = null;
	});

	return mycharts;
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

	charts[i].width = width;
	charts[i].height = height;
	charts[i].refreshCount = 0;
	charts[i].thumbnail = false;

	charts[i].chart = null;
	charts[i].jsondata = null;
	charts[i].datatable = null;
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
		
		if(charts[charts_last_drawn].width == 0 && charts[charts_last_drawn].height == 0) {
			charts[charts_last_drawn].width = width;
			charts[charts_last_drawn].height = height;
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
			charts[charts_last_drawn].width = 0;
			charts[charts_last_drawn].height = 0;
		}
	}
	return 0;
}
