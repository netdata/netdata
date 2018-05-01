package org.firehol.netdata.module.example;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;

import java.util.Collection;
import java.util.stream.Collectors;

import org.firehol.netdata.model.Chart;
import org.firehol.netdata.model.Dimension;
import org.firehol.netdata.module.Module;
import org.junit.Test;

/** Tests for {@link ExampleModule}. */
public class ExampleModuleTest {

	@Test
	public void testLifeCycle() throws Exception {
		// build
		Module.Builder builder = (Module.Builder) Class.forName(ExampleModule.Builder.class.getName()).newInstance();
		Module module = builder.build(null, "example");
		assertEquals("example", module.getName());

		// initialize
		{
			Collection<Chart> charts = module.initialize();
			assertEquals(1, charts.size());

			Chart chart = charts.stream().findFirst().get();
			assertEquals("random_java", chart.getId());

			for (Dimension dimension : chart.getAllDimension()) {
				assertNull(dimension.getCurrentValue());
			}
		}

		// collectValues
		for (int i = 0; i < 10; i++) {
			Collection<Chart> charts = module.collectValues();
			assertEquals(1, charts.size());

			Chart chart = charts.stream().findFirst().get();
			assertEquals("random_java", chart.getId());

			assertArrayEquals(new String[] { "random1", "random2", "random3" },
					chart.getAllDimension().stream().map(Dimension::getId).collect(Collectors.toList()).toArray());
			for (Dimension dimension : chart.getAllDimension()) {
				assertNotNull(dimension.getCurrentValue());
				dimension.setCurrentValue(null); // imitate Printer
			}
		}

		// cleanup
		module.cleanup();
	}
}
