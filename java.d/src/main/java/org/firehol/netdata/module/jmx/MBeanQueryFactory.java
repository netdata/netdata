package org.firehol.netdata.module.jmx;

import org.firehol.netdata.module.jmx.entity.MBeanQueryInfo;
import org.firehol.netdata.module.jmx.query.MBeanDefaultQuery;
import org.firehol.netdata.module.jmx.query.MBeanQuery;
import org.firehol.netdata.module.jmx.query.MBeanQueryLong;

public final class MBeanQueryFactory {

	private MBeanQueryFactory() {
	}

	public static MBeanQuery build(MBeanQueryInfo queryInfo) {
		if (queryInfo.getMBeanAttributeType().isAssignableFrom(Long.class)) {
			return new MBeanQueryLong(queryInfo.getMBeanName(), queryInfo.getMBeanAttribute());
		}

		return new MBeanDefaultQuery(queryInfo.getMBeanName(), queryInfo.getMBeanAttribute(),
				queryInfo.getMBeanAttributeType());
	}
}
