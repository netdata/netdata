package org.firehol.netdata.module.jmx;

import org.firehol.netdata.module.jmx.exception.ClassTypeNotSupportedException;
import org.firehol.netdata.module.jmx.store.MBeanDoubleStore;
import org.firehol.netdata.module.jmx.store.MBeanValueStore;

public class MBeanValueStoreFactory {

	private MBeanValueStoreFactory() {
	}

	public static MBeanValueStore build(Class<?> valueType) throws ClassTypeNotSupportedException {
		if (Double.class.isAssignableFrom(valueType)) {
			return new MBeanDoubleStore();
		}

		throw new ClassTypeNotSupportedException();
	}
}
