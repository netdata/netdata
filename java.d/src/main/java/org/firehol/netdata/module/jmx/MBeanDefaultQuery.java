/*
 * Copyright (C) 2017 Simon Nagl
 *
 * netdata is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package org.firehol.netdata.module.jmx;

import java.util.LinkedList;
import java.util.List;

import javax.management.MBeanServerConnection;
import javax.management.ObjectName;

import org.firehol.netdata.model.Dimension;
import org.firehol.netdata.module.jmx.exception.JmxMBeanServerQueryException;
import org.firehol.netdata.module.jmx.query.MBeanQuery;
import org.firehol.netdata.module.jmx.query.MBeanQueryDimensionMapping;
import org.firehol.netdata.module.jmx.utils.MBeanServerUtils;

import lombok.AccessLevel;
import lombok.Getter;
import lombok.Setter;

/**
 * Technical Object which contains information which attributes of an M(X)Bean
 * we collect and where to store the collected values.
 */
@Getter
@Setter
public class MBeanDefaultQuery extends MBeanQuery {

	private final int LONG_RESOLUTION = 100;

	/**
	 * The Class of the object returned by the query.
	 */
	private Class<?> type;

	@Getter(AccessLevel.NONE)
	private List<Dimension> dimensions = new LinkedList<>();

	public MBeanDefaultQuery(ObjectName name, String attribute, Class<?> attributeType) {
		super(name, attribute);
		this.type = attributeType;
	}

	@Override
	public void addDimension(MBeanQueryDimensionMapping queryInfo) {
		final Dimension dimension = queryInfo.getDimension();

		if (Double.class.isAssignableFrom(type)) {
			dimension.setDivisor(dimension.getDivisor() * this.LONG_RESOLUTION);
		}
		this.dimensions.add(dimension);
	}

	public void query(MBeanServerConnection server) throws JmxMBeanServerQueryException {
		long value = toLong(MBeanServerUtils.getAttribute(server, getName(), getAttribute()));
		for (Dimension dim : dimensions) {
			dim.setCurrentValue(value);
		}
	}

	protected long toLong(Object any) {
		if (any instanceof Integer) {
			return ((Integer) any).longValue();
		} else if (any instanceof Double) {
			double doubleValue = (double) any;
			return (long) (doubleValue * LONG_RESOLUTION);
		} else {
			return (long) any;
		}

	}

}