package org.firehol.netdata.module.example;

import java.util.Collection;
import java.util.HashMap;
import java.util.Map;
import java.util.Random;

import org.firehol.netdata.exception.InitializationException;
import org.firehol.netdata.model.Chart;
import org.firehol.netdata.model.ChartType;
import org.firehol.netdata.model.Dimension;
import org.firehol.netdata.module.Module;
import org.firehol.netdata.plugin.configuration.ConfigurationService;

/**
 * Example netdata {@code java.d} module.
 * 
 * @see <a href=
 *      "https://github.com/firehol/netdata/blob/master/python.d/example.chart.py">example.chart.py
 *      </a>
 */
public class ExampleModule implements Module {

	private final int priority = 90000;

	private final String moduleName;

	private final Map<String, Chart> charts = new HashMap<>();

	private final Random random;

	public ExampleModule(String moduleName) {
		this.moduleName = moduleName;
		this.random = new Random();
	}

	@Override
	public Collection<Chart> initialize() throws InitializationException {
		Chart chart = new Chart();
		chart.setType(getName());
		chart.setId("random_java");
		chart.setName("random");
		chart.setTitle("A random number");
		chart.setUnits("number");
		chart.setFamily("random");
		chart.setContext("random");
		chart.setChartType(ChartType.LINE);
		chart.setPriority(priority);

		chart.getOrAddDimension("random1", id -> new Dimension());

		this.charts.put(chart.getId(), chart);

		return charts.values();
	}

	@Override
	public Collection<Chart> collectValues() {
		for (int i = 1; i < 4; i++) {
			String dimensionId = "random" + i;
			Dimension dimension = charts.get("random_java").getOrAddDimension(dimensionId, id -> new Dimension());
			dimension.setCurrentValue((long) random.nextInt(100));
		}
		return charts.values();
	}

	@Override
	public String getName() {
		return moduleName;
	}

	@Override
	public void cleanup() {
		charts.clear();
	}

	public static class Builder implements Module.Builder {
		@Override
		public Module build(ConfigurationService configurationService, String moduleName) {
			return new ExampleModule(moduleName);
		}
	}
}
