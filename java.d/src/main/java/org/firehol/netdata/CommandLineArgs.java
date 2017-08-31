/*
 * Copyright (C) 2017 Simon Nagl
 *
 * netadata-plugin-java-daemon is free software: you can redistribute it and/or modify
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

package org.firehol.netdata;

import org.firehol.netdata.exception.AssertionException;
import org.firehol.netdata.exception.IllegalCommandLineArumentException;

public final class CommandLineArgs {

	private final String[] args;

	public CommandLineArgs(final String[] args) {
		this.args = args;
	}

	public int getUpdateEveryInSeconds() throws IllegalCommandLineArumentException {
		try {
			assertJustOne();
		} catch (Exception e) {
			throw new IllegalCommandLineArumentException("Wrong number of command lines found.", e);
		}

		try {
			return Integer.valueOf(args[0]);
		} catch (Exception e) {
			throw new IllegalCommandLineArumentException("First command line argument is no integer.", e);
		}

	}

	private void assertJustOne() throws AssertionException {
		if (args.length < 1) {
			throw new AssertionException("Expected just one command line argument. " + args.length + " are present.");
		}
	}

}
