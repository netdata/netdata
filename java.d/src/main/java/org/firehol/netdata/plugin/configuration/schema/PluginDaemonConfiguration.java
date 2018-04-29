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

package org.firehol.netdata.plugin.configuration.schema;

import java.util.Map;

import org.firehol.netdata.module.Module;

import lombok.Getter;
import lombok.Setter;

/**
 * Schema for {@code java.d.conf}.
 */
@Getter
@Setter
public final class PluginDaemonConfiguration {

	/**
	 * Enabled modules mapping module name to the FQCN of a class with a no-arg
	 * constructor implementing {@link Module.Builder}.
	 */
	Map<String, String> modules;

	/**
	 * Log full exception stacktraces into netadata's {@code error.log},
	 * disabled if missing.
	 * 
	 * @see <a href=
	 *      "https://github.com/firehol/netdata/wiki/Log-Files#errorlog">Log-Files
	 *      in Netdata wiki</a>
	 */
	private Boolean logFullStackTraces;
}
