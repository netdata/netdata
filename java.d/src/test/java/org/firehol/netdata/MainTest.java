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

package org.firehol.netdata;

import static org.junit.Assert.assertEquals;
import static org.junit.Assert.fail;

import org.junit.Rule;
import org.junit.Test;
import org.junit.contrib.java.lang.system.ExpectedSystemExit;
import org.junit.contrib.java.lang.system.SystemErrRule;
import org.junit.contrib.java.lang.system.SystemOutRule;

public class MainTest {

	@Rule
	public final ExpectedSystemExit exit = ExpectedSystemExit.none();

	@Rule
	public final SystemOutRule systemOutRule = new SystemOutRule().muteForSuccessfulTests().enableLog();

	@Rule
	public final SystemErrRule systemerrRule = new SystemErrRule().muteForSuccessfulTests();

	@Test
	public void testGetUpdateEveryInSecondsFomCommandLineFailFast() {
		final String[] args = { "3" };

		int updateEvery = Main.getUpdateEveryInSecondsFomCommandLineFailFast(args);

		assertEquals(3, updateEvery);
	}

	@Test
	public void testGetUpdateEveryInSecondsFomCommandLineFailFastFailToMany() {
		exit.expectSystemExitWithStatus(1);
		final String[] args = { "to", "many" };

		Main.getUpdateEveryInSecondsFomCommandLineFailFast(args);
		fail("Should have exit(1)");
	}

	@Test
	public void testGetUpdateEveryInSecondsFomCommandLineFailFastFailNoNumber() {
		exit.expectSystemExitWithStatus(1);
		final String[] args = { "String" };

		Main.getUpdateEveryInSecondsFomCommandLineFailFast(args);
		fail("Should have exit(1)");
	}

	@Test
	public void testExit() throws Exception {
		exit.expectSystemExitWithStatus(1);

		Main.exit("Test");

		assertEquals("DISABLE", systemOutRule.getLog());
	}
}
