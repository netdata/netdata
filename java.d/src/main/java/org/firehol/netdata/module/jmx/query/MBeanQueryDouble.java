package org.firehol.netdata.module.jmx.query;

import java.util.LinkedList;
import java.util.List;

import javax.management.MBeanServerConnection;
import javax.management.ObjectName;

import org.firehol.netdata.model.Dimension;
import org.firehol.netdata.module.jmx.entity.MBeanQueryDimensionMapping;
import org.firehol.netdata.module.jmx.exception.JmxMBeanServerQueryException;
import org.firehol.netdata.module.jmx.utils.MBeanServerUtils;

import lombok.AccessLevel;
import lombok.Getter;

public class MBeanQueryDouble extends MBeanQuery {

	public MBeanQueryDouble(ObjectName name, String attribute) {
		super(name, attribute);
	}

	@Getter(AccessLevel.NONE)
	private List<Dimension> dimensions = new LinkedList<>();

	private final int LONG_RESOLUTION = 100;

	@Override
	public void addDimension(MBeanQueryDimensionMapping queryInfo) {
		final Dimension dimension = queryInfo.getDimension();
		dimension.setDivisor(dimension.getDivisor() * this.LONG_RESOLUTION);
		this.dimensions.add(dimension);
	}

	public void query(MBeanServerConnection server) throws JmxMBeanServerQueryException {

		double doubleValue = (double) MBeanServerUtils.getAttribute(server, getName(), getAttribute());
		long value = (long) (doubleValue * LONG_RESOLUTION);

		for (Dimension dim : dimensions) {
			dim.setCurrentValue(value);
		}
	}
}
