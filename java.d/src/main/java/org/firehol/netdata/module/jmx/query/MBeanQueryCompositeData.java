package org.firehol.netdata.module.jmx.query;

import java.util.LinkedList;
import java.util.List;
import java.util.Map;
import java.util.Map.Entry;
import java.util.TreeMap;

import javax.management.MBeanServerConnection;
import javax.management.ObjectName;
import javax.management.openmbean.CompositeData;

import org.firehol.netdata.model.Dimension;
import org.firehol.netdata.module.jmx.entity.MBeanQueryDimensionMapping;
import org.firehol.netdata.module.jmx.exception.JmxMBeanServerQueryException;
import org.firehol.netdata.module.jmx.utils.MBeanServerUtils;

public class MBeanQueryCompositeData extends MBeanQuery {

	private Map<String, List<Dimension>> allDimensionByCompositeDataKey = new TreeMap<>();

	public MBeanQueryCompositeData(ObjectName name, String attribute) {
		super(name, attribute);
	}

	@Override
	public void addDimension(MBeanQueryDimensionMapping mappingInfo) {
		final String compositeDataKey = mappingInfo.getCompositeDataKey();

		List<Dimension> allDimension = allDimensionByCompositeDataKey.get(compositeDataKey);

		if (allDimension == null) {
			allDimension = new LinkedList<>();
			allDimensionByCompositeDataKey.put(compositeDataKey, allDimension);
		}

		allDimension.add(mappingInfo.getDimension());
	}

	@Override
	public void query(MBeanServerConnection server) throws JmxMBeanServerQueryException {
		CompositeData compositeData = (CompositeData) MBeanServerUtils.getAttribute(server, getName(), getAttribute());

		for (Entry<String, List<Dimension>> dimensionByCompositeDataKey : allDimensionByCompositeDataKey.entrySet()) {
			String compositeDataKey = dimensionByCompositeDataKey.getKey();
			List<Dimension> allDimension = dimensionByCompositeDataKey.getValue();

			long value = (long) compositeData.get(compositeDataKey);

			for (Dimension dimension : allDimension) {
				dimension.setCurrentValue(value);
			}
		}
	}
}
