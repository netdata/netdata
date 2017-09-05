/*
 * Copyright (C) 2017 Simon Nagl
 *
 * netdata is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package org.firehol.netdata.test.utils;

import org.firehol.netdata.model.Chart;
import org.firehol.netdata.model.ChartType;
import org.firehol.netdata.model.Dimension;
import org.firehol.netdata.module.jmx.configuration.JmxChartConfiguration;
import org.firehol.netdata.module.jmx.configuration.JmxDimensionConfiguration;

/**
 * Build standard Test Objects.
 * 
 * A standard Test Object is a instance of a Class where all Properties are set:
 * <ul>
 * <li>Properties with standard values get their standard values</li>
 * <li>String properties are set with the name of the property</li>
 * <li>For enum the most likely value is choosen.</li>
 * <li>Numbers are initialized with 1</li>
 * <li>Booleans get true</li>
 * </ul>
 * 
 * @author Simon Nagl
 */
public abstract class TestObjectBuilder {
	public static Chart buildChart() {
		Chart chart = new Chart();
		chart.setType("type");
		chart.setId("id");
		chart.setName("name");
		chart.setTitle("title");
		chart.setUnits("units");
		chart.setFamily("family");
		chart.setContext("context");
		chart.setChartType(ChartType.LINE);
		return chart;
	}

	public static Dimension buildDimension() {
		Dimension dim = new Dimension();
		dim.setId("id");
		dim.setName("name");
		dim.setHidden(true);
		dim.setCurrentValue(1L);
		return dim;
	}

	public static JmxChartConfiguration buildJmxChartConfiguration() {
		JmxChartConfiguration chartConfig = new JmxChartConfiguration();
		chartConfig.setId("id");
		chartConfig.setTitle("title");
		chartConfig.setFamily("family");
		chartConfig.setUnits("units");
		return chartConfig;
	}

	public static JmxDimensionConfiguration buildJmxDimensionConfiguration() {
		JmxDimensionConfiguration dimensionConfig = new JmxDimensionConfiguration();
		dimensionConfig.setFrom("from");
		dimensionConfig.setValue("value");
		dimensionConfig.setName("name");
		return dimensionConfig;
	}
}
