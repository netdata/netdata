package org.firehol.netdata.module.jmx.store;

import java.util.LinkedList;
import java.util.List;

import org.firehol.netdata.model.Dimension;

public abstract class MBeanValueStore {

	private List<Dimension> dimensions = new LinkedList<>();

	public void addDimension(Dimension dimension) {
		prepareDimension(dimension);
		dimensions.add(dimension);
	}

	protected void prepareDimension(Dimension dimension) {
		// Default implementation does nothing but can be overwritten.
	}

	public void store(Object value) {
		long longValue = toLong(value);

		for (Dimension dimension : dimensions) {
			dimension.setCurrentValue(longValue);
		}
	}

	public abstract long toLong(Object value);
}
