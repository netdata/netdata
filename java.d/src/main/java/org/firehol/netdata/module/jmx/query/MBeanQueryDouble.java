package org.firehol.netdata.module.jmx.query;

import javax.management.MBeanServerConnection;
import javax.management.ObjectName;

import org.firehol.netdata.module.jmx.entity.MBeanQueryDimensionMapping;
import org.firehol.netdata.module.jmx.exception.JmxMBeanServerQueryException;
import org.firehol.netdata.module.jmx.store.MBeanDoubleStore;
import org.firehol.netdata.module.jmx.store.MBeanValueStore;
import org.firehol.netdata.module.jmx.utils.MBeanServerUtils;

import lombok.AccessLevel;
import lombok.Getter;

public class MBeanQueryDouble extends MBeanQuery {

	@Getter(AccessLevel.NONE)
	private MBeanValueStore mBeanValueStore = new MBeanDoubleStore();

	public MBeanQueryDouble(MBeanServerConnection mBeanServerConnection, ObjectName objectName, String string) {
		super(objectName, string, mBeanServerConnection);
	}

	@Override
	public void addDimension(MBeanQueryDimensionMapping queryInfo) {
		mBeanValueStore.addDimension(queryInfo.getDimension());
	}

	public void query() throws JmxMBeanServerQueryException {
		mBeanValueStore.store(MBeanServerUtils.getAttribute(mBeanServer, getName(), getAttribute()));
	}

}
