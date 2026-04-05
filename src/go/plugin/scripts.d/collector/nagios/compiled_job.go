// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/timeperiod"
)

const defaultCollectorUpdateEvery = 10

type compiledJob struct {
	config         JobConfig
	period         *timeperiod.Period
	cadenceWarning string
}

func (j compiledJob) configured() bool {
	return j.config.Plugin != ""
}

func compileCollectorConfig(cfg Config) (compiledJob, error) {
	job, err := cfg.JobConfig.normalized()
	if err != nil {
		return compiledJob{}, err
	}

	periodCfgs := timeperiod.EnsureDefault(append([]timeperiod.Config(nil), cfg.TimePeriods...))
	periodSet, err := timeperiod.Compile(periodCfgs)
	if err != nil {
		return compiledJob{}, err
	}

	period, err := periodSet.Resolve(job.CheckPeriod)
	if err != nil {
		return compiledJob{}, err
	}

	updateEvery := resolveUpdateEvery(cfg.UpdateEvery)

	return compiledJob{
		config:         job,
		period:         period,
		cadenceWarning: cadenceResolutionWarning(job.Name, updateEvery, job.CheckInterval.Duration(), job.RetryInterval.Duration()),
	}, nil
}

func resolveUpdateEvery(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = defaultCollectorUpdateEvery
	}
	return time.Duration(seconds) * time.Second
}

func cadenceResolutionWarning(jobName string, updateEvery, checkInterval, retryInterval time.Duration) string {
	if updateEvery <= 0 {
		return ""
	}
	var requested []string
	if checkInterval > 0 && updateEvery > checkInterval {
		requested = append(requested, fmt.Sprintf("check_interval=%s", checkInterval))
	}
	if retryInterval > 0 && updateEvery > retryInterval {
		requested = append(requested, fmt.Sprintf("retry_interval=%s", retryInterval))
	}
	if len(requested) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"job '%s': update_every (%s) is slower than requested cadence (%s); checks and retries execute on collector ticks, so the effective cadence is limited by update_every",
		jobName,
		updateEvery,
		strings.Join(requested, ", "),
	)
}
