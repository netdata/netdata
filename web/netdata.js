// fix old IE bug with console
if(!window.console){ window.console = {log: function(){} }; }

// Load the Visualization API and the piechart package.
google.load('visualization', '1', {'packages':['corechart']});

function refreshChart(chart) {
	chart.refreshCount++;
	
	if(chart.chart != null) {
		if(chart.chart.getSelection()[0]) return;
		
		if(chart.refreshCount >= 9999999) {
			chart.jsondata = null;
			chart.datatable = null;
			chart.chart.clearChart();
			chart.chart = null;
			chart.refreshCount = 1;
		}
	}
	
	chart.jsondata = $.ajax({
		url: chart.url,
		dataType:"json",
		async: false,
		cache: false
	}).responseText;
	
	if(!chart.jsondata || chart.jsondata.length == 0) return;
	
	// Create our data table out of JSON data loaded from server.
	chart.datatable = new google.visualization.DataTable(chart.jsondata);
	
	// Instantiate and draw our chart, passing in some options.
	if(!chart.chart) {
		console.log('Creating new chart for ' + chart.url);
		chart.chart = new google.visualization.AreaChart(document.getElementById(chart.div));
	}
	
	var width = chart.width;
	var height = chart.height;

	var hAxisTitle = null;
	var vAxisTitle = null;
	if(height >= 250) hAxisTitle = "Time of Day";
	if(width >= 450) vAxisTitle = chart.vtitle;
	
	var title = chart.title;

	var isStacked = false;
	if(chart.name.substring(0, 3) == "tc.") {
		isStacked = true;
		title += " [stacked]";
	}

	var options = {
		width: width,
		height: height,
		title: title,
		isStacked: isStacked,
		hAxis: {title: hAxisTitle},
		vAxis: {title: vAxisTitle, minValue: 10},
		focusTarget: 'category',
		lineWidth: (isStacked)?1:2,
		areaOpacity: (isStacked)?1.0:0.3,
		// animation: {duration: 1000, easing: 'inAndOut'},
	};

	if(chart.chart) chart.chart.draw(chart.datatable, options);
	else console.log('Cannot create chart for ' + chart.url);
}

function cloneJSON(obj) {
    // basic type deep copy
    if (obj === null || obj === undefined || typeof obj !== 'object')  {
        return obj
    }
    // array deep copy
    if (obj instanceof Array) {
        var cloneA = [];
        for (var i = 0; i < obj.length; ++i) {
            cloneA[i] = cloneJSON(obj[i]);
        }              
        return cloneA;
    }                  
    // object deep copy
    var cloneO = {};   
    for (var i in obj) {
        cloneO[i] = cloneJSON(obj[i]);
    }                  
    return cloneO;
}

function loadCharts() {
	var mycharts = new Array();

	$.getJSON('/all.json', function(json) {
		$.each(json.charts, function(i, value) {
			mycharts[i] = $.extend(true, {}, value);

			mycharts[i].div = mycharts[i].name.replace(/\./g,"_");
			mycharts[i].div = mycharts[i].div.replace(/\-/g,"_");
			mycharts[i].div = mycharts[i].div + "_div";

			mycharts[i].width = 0;
			mycharts[i].height = 0;
			mycharts[i].refreshCount = 0;

			mycharts[i].chart = null;
			mycharts[i].jsondata = null;
			mycharts[i].datatable = null;
		});
	});

	console.log("all: " + mycharts.length)
	console.log(mycharts);
	return mycharts;
};

var charts = new Array();
function addChart(name, div, width, height, jsonurl, title, vtitle) {
	var i = charts.length;
	
	console.log('Creating new objects for chart ' + name);
	charts[i] = [];
	charts[i].chart = null;
	charts[i].jsondata = null;
	charts[i].datatable = null;
	charts[i].name = name;
	charts[i].div = div;
	charts[i].url = jsonurl;
	charts[i].title = title;
	charts[i].vtitle = vtitle;
	charts[i].width = width;
	charts[i].height = height;
	charts[i].refreshCount = 0;
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
