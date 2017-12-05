package org.firehol.netdata.module.jmx.store;

import org.firehol.netdata.model.Dimension;

public class MBeanDoubleStore extends MBeanValueStore {

	private final int LONG_RESOLUTION = 100;

	protected void prepareDimension(Dimension dimension) {
		dimension.setDivisor(dimension.getDivisor() * this.LONG_RESOLUTION);
	}

	@Override
	public long toLong(Object value) {
		return (long) ((Double) value * LONG_RESOLUTION);
	}
}
