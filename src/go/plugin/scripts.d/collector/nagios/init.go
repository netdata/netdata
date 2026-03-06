// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

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
	if c.job.configured() {
		return nil
	}

	_, err := c.compileConfiguredJob()
	return err
}

func (c *Collector) compileConfiguredJob() (compiledJob, error) {
	job, err := compileCollectorConfig(c.Config)
	if err != nil {
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
