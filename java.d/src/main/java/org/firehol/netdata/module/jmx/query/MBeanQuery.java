package org.firehol.netdata.module.jmx.query;

import java.util.Objects;

import javax.management.MBeanServerConnection;
import javax.management.ObjectName;

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

	public boolean queryDestinationEquals(MBeanQuery mBeanQuery) {

		if (!Objects.equals(name, mBeanQuery.getName())) {
			return false;
		}

		if (!Objects.equals(attribute, mBeanQuery.getAttribute())) {
			return false;
		}

		return true;
	}

	public abstract void addDimension(MBeanQueryDimensionMapping mappingInfo);

	public abstract void query(MBeanServerConnection server) throws JmxMBeanServerQueryException;

}
