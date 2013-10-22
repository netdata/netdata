	// fix old IE bug with console
	if(!window.console){ window.console = {log: function(){} }; }
	
	// Load the Visualization API and the piechart package.
	google.load('visualization', '1', {'packages':['corechart']});
	
	function refreshChart(index) {
		if(index >= charts.length) return;
		
		var jsonData = $.ajax({
			url: charts_urls[index],
			dataType:"json",
			async: false,
			cache: false
		}).responseText;
		
		if(!jsonData || jsonData.length == 0) return;
		
		// Create our data table out of JSON data loaded from server.
		charts_data[index] = null;
		charts_data[index] = new google.visualization.DataTable(jsonData);
		
		// Instantiate and draw our chart, passing in some options.
		if(!charts[index]) {
			console.log('Creating new chart for ' + charts_urls[index]);
			charts[index] = new google.visualization.AreaChart(document.getElementById(charts_divs[index]));
		}
		
		var width = charts_widths[index];
		if(width == 0) width = (window.innerWidth - 40) / 2;
		if(width <= 10) width = (window.innerWidth - 40) / width;
		if(width < 200) width = 200;
		
		var height = charts_heights[index];
		if(height == 0) height = (window.innerHeight - 20) / 2;
		if(height <= 10) height = (window.innerHeight - 20) / height;
		if(height < 100) height = 100;
		
		var hAxisTitle = null;
		var vAxisTitle = null;
		if(height >= 200) hAxisTitle = "Time of Day";
		if(width >= 400) vAxisTitle = "Bandwidth in kbps";
		
		if(charts[index]) charts[index].draw(charts_data[index], {width: width, height: height, title: charts_titles[index], hAxis: {title: hAxisTitle}, vAxis: {title: vAxisTitle, minValue: 10}});
		else console.log('Cannot create chart for ' + charts_urls[index]);
	}
	
	var charts = new Array();
	var charts_data = new Array();
	var charts_names = new Array();
	var charts_divs = new Array();
	var charts_widths = new Array();
	var charts_heights = new Array();
	var charts_urls = new Array();
	var charts_titles = new Array();
	
	function drawChart(name, div, width, height, jsonurl, title) {
		var i;
		
		for(i = 0; i < charts_names.length; i++) //>
			if(charts_names[i] == name) break;
		
		if(i >= charts_names.length) { //>
			console.log('Creating new objects for chart ' + name);
			charts[i] = null;
			charts_data[i] = null;
			charts_names[i] = name;
			charts_divs[i] = div;
			charts_widths[i] = width;
			charts_heights[i] = height;
			charts_urls[i] = jsonurl;
			charts_titles[i] = title;
		}
		
		try {
			refreshChart(i);
		}
		catch(err) {
			console.log('Cannot create chart for ' + jsonurl);
		}
	}
	
	var charts_last_drawn = 99;
	function refreshCharts(howmany) {
		
		if(charts.length == 0) return;
		
		var h = howmany;
		if(h == 0) h = 1;
		if(h > charts.length) h = charts.length;
		//console.log('Will run for ' + h + ' charts');
		
		var i;
		for(i = 0; i < h; i++) {
			charts_last_drawn++;
			if(charts_last_drawn >= charts.length) charts_last_drawn = 0;
			
			try {
				refreshChart(charts_last_drawn);
			}
			catch(err) {
				console.log('Cannot refresh chart for ' + jsonurl);
			}
		}
		return 0;
	}
