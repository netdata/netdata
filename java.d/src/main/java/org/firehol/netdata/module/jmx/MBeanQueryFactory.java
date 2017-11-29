package org.firehol.netdata.module.jmx;

import javax.management.openmbean.CompositeData;

import org.firehol.netdata.module.jmx.entity.MBeanQueryInfo;
import org.firehol.netdata.module.jmx.query.MBeanDefaultQuery;
import org.firehol.netdata.module.jmx.query.MBeanQuery;
import org.firehol.netdata.module.jmx.query.MBeanQueryCompositeData;
import org.firehol.netdata.module.jmx.query.MBeanQueryDouble;

public final class MBeanQueryFactory {

	private MBeanQueryFactory() {
	}

	public static MBeanQuery build(MBeanQueryInfo queryInfo) {
		if (Double.class.isAssignableFrom(queryInfo.getMBeanAttributeType())) {
			return new MBeanQueryDouble(queryInfo.getMBeanName(), queryInfo.getMBeanAttribute());
		}

		if (CompositeData.class.isAssignableFrom(queryInfo.getMBeanAttributeType())) {
			return new MBeanQueryCompositeData(queryInfo.getMBeanName(), queryInfo.getMBeanAttribute());
		}

		return new MBeanDefaultQuery(queryInfo.getMBeanName(), queryInfo.getMBeanAttribute(),
				queryInfo.getMBeanAttributeType());
	}
}
