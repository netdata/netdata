// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseMQDateTime parses IBM MQ date and time strings into time.Time
// Supports both old format (YYYYMMDD, HHMMSSSS) and new format (YYYY-MM-DD, HH:MM:SS or HH.MM.SS)
//
// CRITICAL: Based on real-world testing, ALL IBM MQ PCF timestamps appear to be UTC:
// - Message timestamps (MQMD PutDate/PutTime): Always UTC
// - Administrative timestamps (start times, alteration times): Also UTC (contrary to some documentation)
//
// Parameters:
//
//	date: Date string in format YYYYMMDD or YYYY-MM-DD
//	timeStr: Time string in format HHMMSSSS or HH:MM:SS or HH.MM.SS
//	isUTC: true for UTC timestamps, false for local time (kept for compatibility)
func ParseMQDateTime(date, timeStr string, isUTC bool) (time.Time, error) {
	if date == "" || timeStr == "" {
		return time.Time{}, fmt.Errorf("empty date or time")
	}

	var parsedTime time.Time
	var err error
	var timezone *time.Location

	// Set timezone based on timestamp type
	if isUTC {
		timezone = time.UTC
	} else {
		timezone = time.Local // Use system local time (queue manager timezone)
	}

	// Try new format first: "YYYY-MM-DD" + "HH:MM:SS" or "HH.MM.SS"
	if len(date) == 10 && date[4] == '-' {
		// New format: "2024-01-15" + "14:30:25" or "14.30.25"
		// Replace dots with colons if present
		cleanTimeStr := strings.ReplaceAll(timeStr, ".", ":")
		datetime := date + "T" + cleanTimeStr

		if isUTC {
			datetime += "Z" // Add UTC suffix for message timestamps
			parsedTime, err = time.Parse("2006-01-02T15:04:05Z", datetime)
		} else {
			// Parse in local timezone for administrative timestamps
			parsedTime, err = time.ParseInLocation("2006-01-02T15:04:05", datetime, timezone)
		}

		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse new format date/time %s + %s (UTC=%v): %v", date, timeStr, isUTC, err)
		}
	} else {
		// Try old format: "YYYYMMDD" + "HHMMSSSS"
		if len(date) != 8 {
			return time.Time{}, fmt.Errorf("invalid date format: %s (expected YYYYMMDD or YYYY-MM-DD)", date)
		}
		if len(timeStr) != 8 {
			return time.Time{}, fmt.Errorf("invalid time format: %s (expected HHMMSSSS or HH:MM:SS)", timeStr)
		}

		// Parse YYYYMMDD
		year, _ := strconv.Atoi(date[0:4])
		month, _ := strconv.Atoi(date[4:6])
		day, _ := strconv.Atoi(date[6:8])

		// Parse HHMMSSSS (HH MM SS SS where last SS is centiseconds)
		hour, _ := strconv.Atoi(timeStr[0:2])
		minute, _ := strconv.Atoi(timeStr[2:4])
		second, _ := strconv.Atoi(timeStr[4:6])
		// Ignore centiseconds for uptime calculation

		parsedTime = time.Date(year, time.Month(month), day, hour, minute, second, 0, timezone)
	}

	return parsedTime, nil
}

// ParseMQMessageDateTime parses IBM MQ message timestamps (always UTC)
// Used for MQMD PutDate/PutTime and other message-related timestamps
func ParseMQMessageDateTime(date, timeStr string) (time.Time, error) {
	return ParseMQDateTime(date, timeStr, true) // Message timestamps are UTC
}

// ParseMQAdminDateTime parses IBM MQ administrative timestamps (UTC)
// CORRECTION: Based on real-world testing, ALL IBM MQ PCF timestamps appear to be UTC
// Used for queue manager start times, listener start times, object alteration times
func ParseMQAdminDateTime(date, timeStr string) (time.Time, error) {
	return ParseMQDateTime(date, timeStr, true) // ALL PCF timestamps are UTC
}

// CalculateUptimeSeconds calculates uptime in seconds from IBM MQ administrative start date and time
// CORRECTION: Based on real-world testing, administrative timestamps are actually UTC
func CalculateUptimeSeconds(startDate, startTime string) (int64, error) {
	parsedTime, err := ParseMQAdminDateTime(startDate, startTime)
	if err != nil {
		return 0, err
	}

	// Calculate uptime as seconds since start time
	uptime := time.Since(parsedTime)
	if uptime < 0 {
		return 0, fmt.Errorf("start time is in the future: %v", parsedTime)
	}

	return int64(uptime.Seconds()), nil
}
