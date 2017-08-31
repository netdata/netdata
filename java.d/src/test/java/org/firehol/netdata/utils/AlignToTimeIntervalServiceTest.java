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

package org.firehol.netdata.utils;

import static org.junit.Assert.assertEquals;

import org.firehol.netdata.testutils.ReflectionUtils;
import org.junit.Rule;
import org.junit.Test;
import org.junit.contrib.java.lang.system.SystemErrRule;

public class AlignToTimeIntervalServiceTest {

	@Rule
	public final SystemErrRule systemErrRule = new SystemErrRule().enableLog();

	@Test
	public void testAlignToTimeIntervalService()
			throws NoSuchFieldException, IllegalAccessException, SecurityException {

		// Test
		AlignToTimeIntervalService service = new AlignToTimeIntervalService(100);

		// Verify
		assertEquals(100L, ReflectionUtils.getPrivateField(service, "intervalInNSec"));
	}

	@Test(timeout = 2000)
	// Just test it does not fail.
	public void testAlignToNextInterval() throws InterruptedException {
		// Build object under test
		AlignToTimeIntervalService service = new AlignToTimeIntervalService(UnitConversion.NANO_PER_PLAIN / 100);

		for (int i = 0; i < 100; i++) {
			service.alignToNextInterval();
		}
	}

}
