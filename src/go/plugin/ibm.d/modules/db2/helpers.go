//go:build cgo
// +build cgo

package db2

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

func safeDSN(dsn string) string {
	if dsn == "" {
		return "<empty>"
	}

	masked := dsn

	if strings.Contains(masked, "PWD=") {
		start := strings.Index(masked, "PWD=")
		if start != -1 {
			end := strings.Index(masked[start:], ";")
			if end == -1 {
				masked = masked[:start] + "PWD=***"
			} else {
				masked = masked[:start] + "PWD=***" + masked[start+end:]
			}
		}
	}

	if strings.Contains(masked, "pwd=") {
		start := strings.Index(masked, "pwd=")
		if start != -1 {
			end := strings.Index(masked[start:], ";")
			if end == -1 {
				masked = masked[:start] + "pwd=***"
			} else {
				masked = masked[:start] + "pwd=***" + masked[start+end:]
			}
		}
	}

	if strings.Contains(masked, "AUTHENTICATION=") {
		start := strings.Index(masked, "AUTHENTICATION=")
		if start != -1 {
			end := strings.Index(masked[start:], ";")
			if end == -1 {
				masked = masked[:start] + "AUTHENTICATION=***"
			} else {
				masked = masked[:start] + "AUTHENTICATION=***" + masked[start+end:]
			}
		}
	}

	return masked
}

func (c *Collector) logOnce(key string, format string, args ...interface{}) {
	if c.disabledMetrics[key] || c.disabledFeatures[key] {
		return
	}

	c.Warningf(format, args...)
	c.disabledMetrics[key] = true
}

func (c *Collector) isDisabled(key string) bool {
	return c.disabledMetrics[key] || c.disabledFeatures[key]
}

func isSQLFeatureError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToUpper(err.Error())
	return strings.Contains(errStr, "SQL0204N") ||
		strings.Contains(errStr, "SQL0206N") ||
		strings.Contains(errStr, "SQL0443N") ||
		strings.Contains(errStr, "SQL0551N") ||
		strings.Contains(errStr, "SQL0707N") ||
		strings.Contains(errStr, "SQLCODE=-204") ||
		strings.Contains(errStr, "SQLCODE=-206") ||
		strings.Contains(errStr, "SQLCODE=-443") ||
		strings.Contains(errStr, "SQLCODE=-551") ||
		strings.Contains(errStr, "UNSUPPORTED COLUMN TYPE")
}

func (c *Collector) collectSingleMetric(ctx context.Context, metricKey string, query string, handler func(value string)) error {
	if c.isDisabled(metricKey) {
		return nil
	}

	db := c.client.DB()
	if db == nil {
		return errors.New("db2 collector: database connection not initialised")
	}

	timeout := time.Duration(c.Timeout)
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var nullValue sql.NullString
	if err := db.QueryRowContext(queryCtx, query).Scan(&nullValue); err != nil {
		if isSQLFeatureError(err) {
			c.logOnce(metricKey, "metric %s not available on this DB2 edition/version: %v", metricKey, err)
			c.disabledMetrics[metricKey] = true
			return nil
		}
		return err
	}

	if nullValue.Valid && nullValue.String != "" {
		handler(nullValue.String)
	}

	return nil
}

func (c *Collector) detectDB2Edition(ctx context.Context) error {
	err := c.collectSingleMetric(ctx, "version_detection_luw", queryDetectVersionLUW, func(value string) {
		c.edition = "LUW"
		c.version = value
		c.parseDB2Version()
		c.Debugf("detected DB2 LUW edition, version: %s", value)
	})
	if err == nil && c.edition != "" {
		c.logVersionInformation()
		return nil
	}

	err = c.collectSingleMetric(ctx, "version_detection_i", queryDetectVersionI, func(value string) {
		c.edition = "i"
		c.version = "DB2 for i"
		c.Debugf("detected DB2 for i (AS/400) edition")
	})
	if err == nil && c.edition != "" {
		c.logVersionInformation()
		return nil
	}

	err = c.collectSingleMetric(ctx, "version_detection_zos", queryDetectVersionZOS, func(value string) {
		c.edition = "z/OS"
		c.version = value
		c.parseDB2Version()
		c.Debugf("detected DB2 for z/OS edition")
	})
	if err == nil && c.edition != "" {
		c.logVersionInformation()
		return nil
	}

	c.edition = "LUW"
	c.version = "Unknown"
	c.Warningf("could not detect DB2 edition, defaulting to LUW")
	c.parseDB2Version()
	c.logVersionInformation()
	return nil
}

func (c *Collector) parseDB2Version() {
	if c.version == "" || c.version == "Unknown" {
		return
	}

	versionStr := c.version
	if strings.Contains(versionStr, "v") {
		parts := strings.Split(versionStr, "v")
		if len(parts) > 1 {
			versionStr = parts[1]
		}
	} else if strings.Contains(versionStr, "V") && strings.Contains(versionStr, "R") {
		versionStr = strings.Replace(versionStr, "V", "", 1)
		versionStr = strings.Replace(versionStr, "R", ".", 1)
		versionStr = strings.Replace(versionStr, "M", ".", 1)
	}

	parts := strings.Split(versionStr, ".")
	if len(parts) >= 2 {
		if major, err := strconv.Atoi(parts[0]); err == nil {
			c.versionMajor = major
		}
		if minor, err := strconv.Atoi(parts[1]); err == nil {
			c.versionMinor = minor
		}
	} else if len(parts) == 1 {
		if major, err := strconv.Atoi(parts[0]); err == nil {
			c.versionMajor = major
		}
	}

	c.Debugf("parsed DB2 version: major=%d, minor=%d", c.versionMajor, c.versionMinor)
}

func (c *Collector) setConfigurationDefaults() {
	if c.CollectDatabaseMetrics.IsAuto() {
		defaultValue := c.edition == "LUW" || c.edition == "Cloud"
		c.CollectDatabaseMetrics = framework.AutoBoolFromBool(defaultValue)
		c.Debugf("CollectDatabaseMetrics not configured, defaulting to %v (edition: %s)", defaultValue, c.edition)
	}

	if c.CollectBufferpoolMetrics.IsAuto() {
		defaultValue := c.edition != "i"
		c.CollectBufferpoolMetrics = framework.AutoBoolFromBool(defaultValue)
		c.Debugf("CollectBufferpoolMetrics not configured, defaulting to %v (edition: %s)", defaultValue, c.edition)
	}

	if c.CollectTablespaceMetrics.IsAuto() {
		defaultValue := true
		c.CollectTablespaceMetrics = framework.AutoBoolFromBool(defaultValue)
		c.Debugf("CollectTablespaceMetrics not configured, defaulting to %v", defaultValue)
	}

	if c.CollectConnectionMetrics.IsAuto() {
		defaultValue := c.edition == "LUW" || (c.edition == "Cloud" && !c.isDisabled("connection_instances"))
		c.CollectConnectionMetrics = framework.AutoBoolFromBool(defaultValue)
		c.Debugf("CollectConnectionMetrics not configured, defaulting to %v (edition: %s)", defaultValue, c.edition)
	}

	if c.CollectLockMetrics.IsAuto() {
		defaultValue := true
		c.CollectLockMetrics = framework.AutoBoolFromBool(defaultValue)
		c.Debugf("CollectLockMetrics not configured, defaulting to %v", defaultValue)
	}

	if c.CollectTableMetrics.IsAuto() {
		defaultValue := false
		c.CollectTableMetrics = framework.AutoBoolFromBool(defaultValue)
		c.Debugf("CollectTableMetrics not configured, defaulting to %v", defaultValue)
	}

	if c.CollectIndexMetrics.IsAuto() {
		defaultValue := false
		c.CollectIndexMetrics = framework.AutoBoolFromBool(defaultValue)
		c.Debugf("CollectIndexMetrics not configured, defaulting to %v", defaultValue)
	}

	c.Infof("Configuration after defaults: DB=%t Bufferpool=%t Tablespace=%t Connection=%t",
		c.CollectDatabaseMetrics.IsEnabled(),
		c.CollectBufferpoolMetrics.IsEnabled(),
		c.CollectTablespaceMetrics.IsEnabled(),
		c.CollectConnectionMetrics.IsEnabled())
	c.Infof("Lock=%t Table=%t Index=%t Memory=%v Wait=%v TableIO=%v",
		c.CollectLockMetrics.IsEnabled(),
		c.CollectTableMetrics.IsEnabled(),
		c.CollectIndexMetrics.IsEnabled(),
		c.CollectMemoryMetrics,
		c.CollectWaitMetrics,
		c.CollectTableIOMetrics)
}

func (c *Collector) logVersionInformation() {
	c.parseDB2Version()

	c.Infof("DB2 %s edition detected - collector will attempt all configured features with graceful error handling", c.edition)

	switch c.edition {
	case "i":
		c.Infof("DB2 for i (AS/400) typically has limited SYSIBMADM view support")
	case "z/OS":
		c.Infof("DB2 for z/OS typically has different monitoring capabilities than LUW")
	case "Cloud":
		c.Infof("Db2 on Cloud typically restricts some system-level metrics compared to standard DB2 LUW")
	case "LUW":
		if c.versionMajor > 0 {
			c.Infof("DB2 LUW %c.%d detected", c.versionMajor, c.versionMinor)
		} else {
			c.Infof("DB2 LUW version unknown - assuming broad feature availability")
		}
	default:
		c.Infof("Unknown DB2 edition '%s' - assuming LUW-like capabilities", c.edition)
	}

	c.Infof("Note: admin configuration takes precedence - all enabled features will be attempted")
}

func (c *Collector) detectColumnOrganizedSupport(ctx context.Context) {
	if c.edition == "i" {
		c.Infof("Column-organized tables not available on DB2 for i")
		return
	}

	if c.versionMajor > 0 && c.versionMajor < 10 {
		c.Infof("Column-organized tables require DB2 10.5+, current version %c.%d", c.versionMajor, c.versionMinor)
		return
	}

	if c.versionMajor == 10 && c.versionMinor < 5 {
		c.Infof("Column-organized tables require DB2 10.5+, current version %c.%d", c.versionMajor, c.versionMinor)
		return
	}

	c.Debugf("Column-organized buffer pool metrics expected to be available (DB2 %c.%d)", c.versionMajor, c.versionMinor)
}

func cleanName(name string) string {
	replacer := strings.NewReplacer(
		" ", "_",
		".", "_",
		"-", "_",
		"/", "_",
		":", "_",
		"=", "_",
	)
	return strings.ToLower(replacer.Replace(name))
}
