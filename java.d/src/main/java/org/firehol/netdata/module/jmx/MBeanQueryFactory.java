package org.firehol.netdata.module.jmx;

import org.firehol.netdata.module.jmx.query.MBeanQuery;

public final class MBeanQueryFactory {

	private MBeanQueryFactory() {
	}

	public static MBeanQuery build(MBeanQueryInfo queryInfo) {
		return new MBeanDefaultQuery(queryInfo.getMBeanName(), queryInfo.getMBeanAttribute(),
				queryInfo.getMBeanAttributeType());
	}
}
