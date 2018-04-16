package org.firehol.netdata.module.jmx.store;

public class MBeanLongStore extends MBeanValueStore {

	@Override
	public long toLong(Object value) {
		return (long) ((Long) value);
	}
}
