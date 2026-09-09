// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

	pluginPath, args, err := rewriteScriptCommand(job.config.Plugin, job.config.Args)
	if err != nil {
		return compiledJob{}, fmt.Errorf("job '%s': %w", job.config.Name, err)
	}
	job.config.Plugin = pluginPath
	job.config.Args = args

	validatedPath, err := c.validatePlugin(job.config.Plugin)
	if err != nil {
		return compiledJob{}, fmt.Errorf("job '%s': %w", job.config.Name, err)
	}
	job.config.Plugin = validatedPath

	if runtime.GOOS != "windows" && isKnownInterpreter(job.config.Plugin) {
		c.Warningf("job '%s': plugin '%s' appears to be an interpreter; "+
			"Netdata validates the interpreter binary but cannot verify scripts passed in args — "+
			"ensure scripts are root-owned and not writable by group/others",
			job.config.Name, job.config.Plugin)
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

var knownInterpreters = []string{
	"bash", "sh", "dash", "zsh", "ksh", "csh", "tcsh", "fish",
	"python", "python2", "python3",
	"perl", "ruby", "lua",
	"powershell", "pwsh",
	"cmd",
	"node",
	"env",
	"php",
	"tclsh", "wish", "expect",
}

func isKnownInterpreter(pluginPath string) bool {
	base := filepath.Base(pluginPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	return slices.Contains(knownInterpreters, strings.ToLower(name))
}
