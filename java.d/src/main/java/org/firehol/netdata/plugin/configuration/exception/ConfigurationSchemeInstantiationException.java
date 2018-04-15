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

package org.firehol.netdata.plugin.configuration.exception;

import javax.naming.ConfigurationException;

public class ConfigurationSchemeInstantiationException extends ConfigurationException {
	private static final long serialVersionUID = -5538037492659066003L;

	public ConfigurationSchemeInstantiationException() {
	}

	public ConfigurationSchemeInstantiationException(String explanation) {
		super(explanation);
	}

	public ConfigurationSchemeInstantiationException(String explanation, Throwable cause) {
		this(explanation);
		this.initCause(cause);
	}
}
