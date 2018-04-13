package org.firehol.netdata.module.jmx.store;

import org.firehol.netdata.model.Dimension;

public class MBeanLongStore extends MBeanValueStore {

	protected void prepareDimension(Dimension dimension) {
		dimension.setDivisor(dimension.getDivisor());
	}

	@Override
	public long toLong(Object value) {
		return (long) ((Long) value);
	}
}
