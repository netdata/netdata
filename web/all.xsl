<?xml version="1.0" encoding="utf-8"?>
<xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">

<xsl:template match="/">
<html>
<style>
	* {font-family:Arial}
	div {float: left; margin: 0 0 0 0; }
</style>

<head>
	<!--Load the AJAX API-->
	<script type="text/javascript" src="https://www.google.com/jsapi"></script>
	<script src="//ajax.googleapis.com/ajax/libs/jquery/1.10.2/jquery.min.js"></script>
	<script type="text/javascript" src="netdata.js"></script>
	<script type="text/javascript">

	<!--
	<xsl:variable name="unique-list" select="//graph/type[not(.=following::type)]" />
	<xsl:for-each select="$unique-list">
	alert('<xsl:value-of select="." />');
	</xsl:for-each>
	-->
	
	// Set a callback to run when the Google Visualization API is loaded.
	google.setOnLoadCallback(drawCharts);

	function drawCharts() {
		<xsl:for-each select="catalog/graph">
		addChart('<xsl:value-of select="name"/>', 'div_graph_<xsl:value-of select="position()"/>', 0, 0, "<xsl:value-of select="dataurl"/>/0/1/average", "<xsl:value-of select="title"/>", "<xsl:value-of select="vtitle"/>");
		</xsl:for-each>
		refreshCharts(999999);
	}

	var refreshCount = 0;
	function myChartsRefresh() {
		refreshCount += 2;
		if(refreshCount > 500) location.reload();

		// refresh up to 2 charts per second
		refreshCharts(2);
	}

	setInterval(myChartsRefresh, 1000);

	//window.onresize = function(event) {
	//	refreshCharts(999999);
	//};

	</script> 
</head>
<body>
	<xsl:for-each select="catalog/graph">
	<div>
		<div>
			<xsl:attribute name="id">div_graph_<xsl:value-of select="position()"/></xsl:attribute>
		</div>
	</div>
	</xsl:for-each>

</body>
</html>
</xsl:template>
</xsl:stylesheet>
