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

package org.firehol.netdata.utils;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

public class StringUtilsTest {

	@Test
	public void testIsBlankNull() {
		assertTrue(StringUtils.isBlank(null));
	}

	@Test
	public void testIsBlankEmpty() {
		assertTrue(StringUtils.isBlank(""));
	}

	@Test
	public void testIsBlankWhitespace() {
		assertTrue(StringUtils.isBlank(" "));
	}

	@Test
	public void testIsBlankEmptyFilled() {
		assertFalse(StringUtils.isBlank("bob"));
	}

	@Test
	public void testIsBlankEmptyFilledWithWhitespace() {
		assertFalse(StringUtils.isBlank("  bob  "));
	}

}
