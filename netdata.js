	// fix old IE bug with console
	if(!window.console){ window.console = {log: function(){} }; }
	
	// Load the Visualization API and the piechart package.
	google.load('visualization', '1', {'packages':['corechart']});
	
	function refreshChart(index, div, width, height, jsonurl, title) {
		var jsonData = $.ajax({
			url: jsonurl,
			dataType:"json",
			async: false,
			cache: false
		}).responseText;
		
		// Create our data table out of JSON data loaded from server.
		charts_data[index] = new google.visualization.DataTable(jsonData);
		
		// Instantiate and draw our chart, passing in some options.
		if(!charts[index]) {
			console.log('Creating new chart for ' + jsonurl);
			charts[index] = new google.visualization.AreaChart(document.getElementById(div));
		}
		if(charts[index]) charts[index].draw(charts_data[index], {width: width, height: height, title: title, hAxis: {title: "Time of Day"}, vAxis: {title: "Bandwidth in kbps", minValue: 200}});
		else console.log('Cannot create chart for ' + jsonurl);
	}
	
	var charts = new Array();
	var charts_data = new Array();
	var charts_names = new Array();
	
	function drawChart(name, div, width, height, jsonurl, title) {
		var i;
		
		for(i = 0; i < charts_names.length; i++) //>
			if(charts_names[i] == name) break;
		
		if(i >= charts_names.length) { //>
			console.log('Creating new objects for chart ' + name);
			charts[i] = null;
			charts_data[i] = null;
			charts_names[i] = name;
		}
		
		try {
			refreshChart(i, div, width, height, jsonurl, title);
		}
		catch(err) {
			console.log('Cannot create chart for ' + jsonurl);
		}
	}

