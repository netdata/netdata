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
func ParseMQDateTime(startDate, startTime string) (time.Time, error) {
	if startDate == "" || startTime == "" {
		return time.Time{}, fmt.Errorf("empty start date or time")
	}
	
	var parsedTime time.Time
	var err error
	
	// Try new format first: "YYYY-MM-DD" + "HH:MM:SS" or "HH.MM.SS"
	if len(startDate) == 10 && startDate[4] == '-' {
		// New format: "2024-01-15" + "14:30:25" or "14.30.25"
		// Replace dots with colons if present
		timeStr := strings.ReplaceAll(startTime, ".", ":")
		datetime := startDate + "T" + timeStr + "Z"
		parsedTime, err = time.Parse("2006-01-02T15:04:05Z", datetime)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse new format date/time %s + %s: %v", startDate, startTime, err)
		}
	} else {
		// Try old format: "YYYYMMDD" + "HHMMSSSS" 
		if len(startDate) != 8 {
			return time.Time{}, fmt.Errorf("invalid date format: %s (expected YYYYMMDD or YYYY-MM-DD)", startDate)
		}
		if len(startTime) != 8 {
			return time.Time{}, fmt.Errorf("invalid time format: %s (expected HHMMSSSS or HH:MM:SS)", startTime)
		}
		
		// Parse YYYYMMDD
		year, _ := strconv.Atoi(startDate[0:4])
		month, _ := strconv.Atoi(startDate[4:6])
		day, _ := strconv.Atoi(startDate[6:8])
		
		// Parse HHMMSSSS (HH MM SS SS where last SS is centiseconds)
		hour, _ := strconv.Atoi(startTime[0:2])
		minute, _ := strconv.Atoi(startTime[2:4])
		second, _ := strconv.Atoi(startTime[4:6])
		// Ignore centiseconds for uptime calculation
		
		parsedTime = time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	}
	
	return parsedTime, nil
}

// CalculateUptimeSeconds calculates uptime in seconds from IBM MQ start date and time
func CalculateUptimeSeconds(startDate, startTime string) (int64, error) {
	parsedTime, err := ParseMQDateTime(startDate, startTime)
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