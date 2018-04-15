package org.firehol.netdata.module.jmx.query;

import java.util.Objects;

import javax.management.MBeanServerConnection;
import javax.management.ObjectName;

import org.firehol.netdata.exception.InitializationException;
import org.firehol.netdata.module.jmx.entity.MBeanQueryDimensionMapping;
import org.firehol.netdata.module.jmx.exception.JmxMBeanServerQueryException;

import lombok.AllArgsConstructor;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
@AllArgsConstructor
public abstract class MBeanQuery {
	private ObjectName name;

	private String attribute;

	protected MBeanServerConnection mBeanServer;

	public boolean queryDestinationEquals(MBeanQuery mBeanQuery) {
		return Objects.equals(name, mBeanQuery.getName()) && Objects.equals(attribute, mBeanQuery.getAttribute());
	}

	public abstract void addDimension(MBeanQueryDimensionMapping mappingInfo) throws InitializationException;

	public abstract void query() throws JmxMBeanServerQueryException;

}
