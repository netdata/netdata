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

import java.util.logging.Logger;

public class AlignToTimeIntervalService {
	private Logger log = Logger.getLogger("org.firehol.netdata.utils.aligntotimeintervalservice");

	private long intervalInNSec;
	private long lastTimestamp;

	public AlignToTimeIntervalService(long intervalInNSec) {
		this.intervalInNSec = intervalInNSec;
		this.lastTimestamp = ClockService.nowMonotonicNSec();
	}

	public long alignToNextInterval() {
		long now = ClockService.nowMonotonicNSec();
		long next = now - (now % intervalInNSec) + intervalInNSec;

		while (now < next) {
			try {
				Thread.sleep((next - now) / UnitConversion.MILI_PER_NANO,
						(int) (next - now) % UnitConversion.MILI_PER_NANO);
			} catch (InterruptedException e) {
				log.warning("Interrupted while waiting for next tick.");
				// We try again here. The worst might happen is a busy wait
				// instead of sleeping.
			}
			now = ClockService.nowMonotonicNSec();
		}

		long delta = now - lastTimestamp;
		lastTimestamp = now;
		if (delta / intervalInNSec > 1) {
			log.warning("At least one tick missed since last call of alignToNextInterval()");
		}
		return delta;
	}

}
