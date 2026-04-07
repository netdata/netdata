// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"fmt"
	"os"
	"runtime"
)

func (c *Collector) initCollector() error {
	job, err := c.compileConfiguredJob()
	if err != nil {
		return err
	}
	c.job = job
	c.state = newCollectState(c.now(), job.config)
	return nil
}

func (c *Collector) checkCollector() error {
	_, err := c.compileConfiguredJob()
	return err
}

func (c *Collector) compileConfiguredJob() (compiledJob, error) {
	job, err := compileCollectorConfig(c.Config)
	if err != nil {
		return compiledJob{}, err
	}
	if err := validateConfiguredPlugin(job.config); err != nil {
		return compiledJob{}, err
	}
	c.warnCadenceResolution(job)
	return job, nil
}

func (c *Collector) warnCadenceResolution(job compiledJob) {
	if job.cadenceWarning == "" || job.cadenceWarning == c.cadenceWarning {
		return
	}
	c.Warningf("%s", job.cadenceWarning)
	c.cadenceWarning = job.cadenceWarning
}

func validateConfiguredPlugin(job JobConfig) error {
	info, err := os.Stat(job.Plugin)
	if err != nil {
		return fmt.Errorf("job '%s': plugin path '%s' stat error: %w", job.Name, job.Plugin, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("job '%s': plugin path '%s' must be a regular file", job.Name, job.Plugin)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("job '%s': plugin path '%s' must be executable", job.Name, job.Plugin)
	}
	return nil
}
