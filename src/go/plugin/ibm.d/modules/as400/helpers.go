//go:build cgo
// +build cgo

package as400

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	as400proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/as400"
)

func cleanName(name string) string {
	r := strings.NewReplacer(
		" ", "_",
		".", "_",
		"-", "_",
		"/", "_",
		":", "_",
		"=", "_",
	)
	return strings.ToLower(r.Replace(name))
}

func (c *Collector) logOnce(key string, format string, args ...interface{}) {
	if c.disabled[key] {
		return
	}
	c.Warningf(format, args...)
	c.disabled[key] = true
}

func (c *Collector) isDisabled(key string) bool {
	return c.disabled[key]
}

func isSQLFeatureError(err error) bool {
	return as400proto.IsFeatureError(err)
}

func isSQLTemporaryError(err error) bool {
	return as400proto.IsTemporaryError(err)
}

func (c *Collector) collectSingleMetric(ctx context.Context, metricKey string, query string, handler func(value string)) error {
	if c.isDisabled(metricKey) {
		return nil
	}

	err := c.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if value != "" {
			handler(value)
		}
	})

	if err != nil && isSQLFeatureError(err) {
		c.logOnce(metricKey, "metric %s not available on this IBM i version: %v", metricKey, err)
		c.disabled[metricKey] = true
		return nil
	}

	return err
}

func (c *Collector) detectIBMiVersion(ctx context.Context) error {
	var version, release string
	versionDetected := false

	err := c.doQuery(ctx, queryIBMiVersion, func(column, value string, lineEnd bool) {
		switch column {
		case "OS_NAME":
			// ignore
		case "OS_VERSION":
			version = strings.TrimSpace(value)
		case "OS_RELEASE":
			release = strings.TrimSpace(value)
		}
	})

	if err == nil && version != "" && release != "" {
		versionDetected = true
		c.osVersion = fmt.Sprintf("%s.%s", version, release)
		c.Debugf("detected IBM i version from ENV_SYS_INFO: %s", c.osVersion)
	} else if err != nil {
		c.Debugf("ENV_SYS_INFO query failed: %v, trying fallback method", err)

		err = c.doQuery(ctx, queryIBMiVersionDataArea, func(column, value string, lineEnd bool) {
			if column == "VERSION" {
				dataAreaValue := strings.TrimSpace(value)
				if len(dataAreaValue) >= 6 {
					c.osVersion = dataAreaValue
					versionDetected = true
					c.Debugf("detected IBM i version from data area: %s", c.osVersion)
				}
			}
		})

		if err != nil {
			c.Warningf("failed to detect IBM i version from data area: %v", err)
		}
	}

	if !versionDetected {
		c.osVersion = "7.4"
		c.Warningf("could not detect IBM i version, using default: %s", c.osVersion)
	}

	c.parseIBMiVersion()
	return nil
}

func (c *Collector) detectAvailableFeatures(ctx context.Context) {
	c.disabled["active_job_info"] = true
	c.disabled["ifs_object_statistics"] = true
}

func (c *Collector) collectSystemInfo(ctx context.Context) {
	c.systemName = "Unknown"

	_ = c.collectSingleMetric(ctx, "serial_number", querySerialNumber, func(value string) {
		c.serialNumber = strings.TrimSpace(value)
		c.Debugf("detected serial number: %s", c.serialNumber)
	})

	_ = c.collectSingleMetric(ctx, "system_model", querySystemModel, func(value string) {
		c.model = strings.TrimSpace(value)
		c.Debugf("detected system model: %s", c.model)
	})

	err := c.doQuery(ctx, queryTechnologyRefresh, func(column, value string, lineEnd bool) {
		if column == "TR_LEVEL" && value != "" {
			trLevel := strings.TrimSpace(value)
			if trLevel != "" {
				c.technologyRefresh = fmt.Sprintf("TR%s", trLevel)
				c.Debugf("detected Technology Refresh level: %s", c.technologyRefresh)
			}
		}
	})
	if err != nil {
		c.Debugf("failed to detect Technology Refresh level: %v", err)
	}

	if c.systemName == "" {
		c.systemName = "Unknown"
	}
	if c.serialNumber == "" {
		c.serialNumber = "Unknown"
	}
	if c.model == "" {
		c.model = "Unknown"
	}
	if c.osVersion == "" {
		c.osVersion = "Unknown"
	}
	if c.technologyRefresh == "" {
		c.technologyRefresh = "Unknown"
	}
}

func (c *Collector) parseIBMiVersion() {
	if c.osVersion == "" || c.osVersion == "Unknown" {
		return
	}

	versionStr := strings.ToUpper(strings.TrimSpace(c.osVersion))
	if idx := strings.IndexAny(versionStr, " L"); idx > 0 {
		versionStr = versionStr[:idx]
	}
	versionStr = strings.ReplaceAll(versionStr, " ", "")

	if strings.HasPrefix(versionStr, "V") {
		versionStr = versionStr[1:]
		if idx := strings.Index(versionStr, "R"); idx > 0 {
			if major, err := strconv.Atoi(versionStr[:idx]); err == nil {
				c.versionMajor = major
			}
			remainder := versionStr[idx+1:]
			if mIdx := strings.Index(remainder, "M"); mIdx > 0 {
				if release, err := strconv.Atoi(remainder[:mIdx]); err == nil {
					c.versionRelease = release
				}
				modStr := remainder[mIdx+1:]
				modNum := ""
				for _, ch := range modStr {
					if ch >= '0' && ch <= '9' {
						modNum += string(ch)
					} else {
						break
					}
				}
				if modNum != "" {
					if mod, err := strconv.Atoi(modNum); err == nil {
						c.versionMod = mod
					}
				}
			} else {
				if release, err := strconv.Atoi(remainder); err == nil {
					c.versionRelease = release
				}
			}
		}
	} else {
		parts := strings.Split(versionStr, ".")
		if len(parts) >= 2 {
			if major, err := strconv.Atoi(parts[0]); err == nil {
				c.versionMajor = major
			}
			if release, err := strconv.Atoi(parts[1]); err == nil {
				c.versionRelease = release
			}
			if len(parts) >= 3 {
				if mod, err := strconv.Atoi(parts[2]); err == nil {
					c.versionMod = mod
				}
			}
		} else if len(parts) == 1 {
			if major, err := strconv.Atoi(parts[0]); err == nil {
				c.versionMajor = major
			}
		}
	}

	c.Debugf("parsed IBM i version: major=%d, release=%d, mod=%d", c.versionMajor, c.versionRelease, c.versionMod)
}

func (c *Collector) logVersionInformation() {
	if c.versionMajor > 0 {
		c.Infof("IBM i %d.%d detected - collector will attempt all configured features with graceful error handling", c.versionMajor, c.versionRelease)

		if c.versionMajor >= 7 {
			if c.versionRelease >= 5 {
				c.Infof("IBM i 7.5+ typically supports all collector features")
			} else if c.versionRelease >= 3 {
				c.Infof("IBM i 7.3+ typically supports ACTIVE_JOB_INFO and IFS_OBJECT_STATISTICS")
			} else if c.versionRelease >= 2 {
				c.Infof("IBM i 7.2+ typically supports MESSAGE_QUEUE_INFO and JOB_QUEUE_ENTRIES")
			} else {
				c.Infof("IBM i 7.1+ has basic SQL services - some advanced features may not be available")
			}
		} else {
			c.Infof("IBM i %d.x has limited SQL services - many features may not be available", c.versionMajor)
		}

		c.Infof("Note: admin configuration takes precedence - all enabled features will be attempted regardless of version")
	} else {
		c.Infof("IBM i version unknown - collector will attempt all configured features with graceful error handling")
	}
}

func (c *Collector) setConfigurationDefaults() {
	jobQueuesDefault := c.versionMajor >= 7 && c.versionRelease >= 2
	c.CollectDiskMetrics = c.CollectDiskMetrics.WithDefault(true)
	c.CollectSubsystemMetrics = c.CollectSubsystemMetrics.WithDefault(true)
	c.CollectJobQueueMetrics = c.CollectJobQueueMetrics.WithDefault(jobQueuesDefault)
	c.CollectActiveJobs = c.CollectActiveJobs.WithDefault(false)
	c.CollectMessageQueueMetrics = c.CollectMessageQueueMetrics.WithDefault(true)
	c.CollectOutputQueueMetrics = c.CollectOutputQueueMetrics.WithDefault(true)
	c.CollectHTTPServerMetrics = c.CollectHTTPServerMetrics.WithDefault(true)
	c.CollectPlanCacheMetrics = c.CollectPlanCacheMetrics.WithDefault(true)

	if c.MaxMessageQueues <= 0 {
		c.MaxMessageQueues = messageQueueLimit
	}

	if c.MaxOutputQueues <= 0 {
		c.MaxOutputQueues = outputQueueLimit
	}

	c.Infof("Configuration after defaults: DiskMetrics=%t, SubsystemMetrics=%t, JobQueueMetrics=%t, MessageQueues=%t, OutputQueues=%t, ActiveJobs=%t, HTTPServer=%t, PlanCache=%t",
		c.CollectDiskMetrics.IsEnabled(),
		c.CollectSubsystemMetrics.IsEnabled(),
		c.CollectJobQueueMetrics.IsEnabled(),
		c.CollectMessageQueueMetrics.IsEnabled(),
		c.CollectOutputQueueMetrics.IsEnabled(),
		c.CollectActiveJobs.IsEnabled(),
		c.CollectHTTPServerMetrics.IsEnabled(),
		c.CollectPlanCacheMetrics.IsEnabled())
}

func (c *Collector) systemStatusQuery() string {
	if c.ResetStatistics {
		return querySystemStatusReset
	}
	return querySystemStatusNoReset
}

func (c *Collector) memoryPoolQuery() string {
	if c.ResetStatistics {
		return queryMemoryPoolsReset
	}
	return queryMemoryPoolsNoReset
}
