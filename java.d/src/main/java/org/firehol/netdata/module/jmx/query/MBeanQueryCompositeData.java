package org.firehol.netdata.module.jmx.query;

import java.util.Map;
import java.util.Map.Entry;
import java.util.TreeMap;

import javax.management.MBeanServerConnection;
import javax.management.ObjectName;
import javax.management.openmbean.CompositeData;

import org.firehol.netdata.exception.InitializationException;
import org.firehol.netdata.module.jmx.MBeanValueStoreFactory;
import org.firehol.netdata.module.jmx.entity.MBeanQueryDimensionMapping;
import org.firehol.netdata.module.jmx.exception.ClassTypeNotSupportedException;
import org.firehol.netdata.module.jmx.exception.JmxMBeanServerQueryException;
import org.firehol.netdata.module.jmx.store.MBeanValueStore;
import org.firehol.netdata.module.jmx.utils.MBeanServerUtils;

public class MBeanQueryCompositeData extends MBeanQuery {

	private Map<String, MBeanValueStore> valueStoreByCompositeDataKey = new TreeMap<>();

	public MBeanQueryCompositeData(ObjectName name, String attribute, MBeanServerConnection mBeanServer) {
		super(name, attribute, mBeanServer);
	}

	@Override
	public void addDimension(MBeanQueryDimensionMapping mappingInfo) throws InitializationException {
		final String compositeDataKey = mappingInfo.getCompositeDataKey();

		MBeanValueStore valueStore = valueStoreByCompositeDataKey.get(compositeDataKey);

		if (valueStore == null) {
			CompositeData compositeData;
			try {
				compositeData = (CompositeData) MBeanServerUtils.getAttribute(mBeanServer, getName(), getAttribute());
			} catch (JmxMBeanServerQueryException e) {
				throw new InitializationException("Could not query for attribute.", e);
			}

			Object value = compositeData.get(compositeDataKey);

			try {
				valueStore = MBeanValueStoreFactory.build(value.getClass());
			} catch (ClassTypeNotSupportedException e) {
				throw new InitializationException(e);
			}

			valueStoreByCompositeDataKey.put(compositeDataKey, valueStore);
		}

		valueStore.addDimension(mappingInfo.getDimension());
	}

	@Override
	public void query() throws JmxMBeanServerQueryException {
		CompositeData compositeData = (CompositeData) MBeanServerUtils.getAttribute(mBeanServer, getName(),
				getAttribute());

		for (Entry<String, MBeanValueStore> dimensionByCompositeDataKey : valueStoreByCompositeDataKey.entrySet()) {
			String compositeDataKey = dimensionByCompositeDataKey.getKey();
			MBeanValueStore valueStore = dimensionByCompositeDataKey.getValue();

			valueStore.store(compositeData.get(compositeDataKey));
		}
	}
}
