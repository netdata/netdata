// SPDX-License-Identifier: GPL-3.0-or-later

package vnodectl

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

const (
	dyncfgVnodeIDf  = "%s:vnode"
	dyncfgVnodePath = "/collectors/%s/Vnodes"
)

type Options struct {
	Logger  *logger.Logger
	API     *dyncfg.Responder
	Plugin  string
	Initial map[string]*vnodes.VirtualNode

	AffectedJobs     func(string) []string
	ApplyVnodeUpdate func(string, *vnodes.VirtualNode)
}

type Controller struct {
	*logger.Logger

	api        *dyncfg.Responder
	pluginName string
	store      *vnodeStore

	affectedJobs     func(string) []string
	applyVnodeUpdate func(string, *vnodes.VirtualNode)
}

func New(opts Options) *Controller {
	log := opts.Logger
	if log == nil {
		log = logger.New()
	}

	return &Controller{
		Logger:           log,
		api:              opts.API,
		pluginName:       opts.Plugin,
		store:            newVnodeStore(opts.Initial),
		affectedJobs:     opts.AffectedJobs,
		applyVnodeUpdate: opts.ApplyVnodeUpdate,
	}
}

func (c *Controller) Prefix() string {
	return fmt.Sprintf(dyncfgVnodeIDf, c.pluginName)
}

func (c *Controller) SetAPI(api *dyncfg.Responder) {
	if api == nil {
		// Nil means "keep the current responder" rather than clearing output wiring.
		return
	}
	c.api = api
}

func (c *Controller) Lookup(name string) (*vnodes.VirtualNode, bool) {
	if c.store == nil {
		return nil, false
	}
	return c.store.Lookup(name)
}

func (c *Controller) CreateTemplates() {
	if c.api == nil {
		return
	}
	c.api.ConfigCreate(netdataapi.ConfigOpts{
		ID:                c.Prefix(),
		Status:            dyncfg.StatusAccepted.String(),
		ConfigType:        dyncfg.ConfigTypeTemplate.String(),
		Path:              fmt.Sprintf(dyncfgVnodePath, c.pluginName),
		SourceType:        "internal",
		Source:            "internal",
		SupportedCommands: dyncfgVnodeModCmds(),
	})
}

func (c *Controller) PublishExisting(status dyncfg.Status) {
	if c.store == nil {
		return
	}
	c.store.ForEach(func(cfg *vnodes.VirtualNode) bool {
		c.createJob(cfg, status)
		return true
	})
}

func (c *Controller) configID(name string) string {
	return fmt.Sprintf("%s:%s", c.Prefix(), name)
}

func (c *Controller) createJob(cfg *vnodes.VirtualNode, status dyncfg.Status) {
	if c.api == nil || cfg == nil {
		return
	}
	c.api.ConfigCreate(netdataapi.ConfigOpts{
		ID:                c.configID(cfg.Name),
		Status:            status.String(),
		ConfigType:        dyncfg.ConfigTypeJob.String(),
		Path:              fmt.Sprintf(dyncfgVnodePath, c.pluginName),
		SourceType:        cfg.SourceType,
		Source:            cfg.Source,
		SupportedCommands: dyncfgVnodeJobCmds(cfg.SourceType == confgroup.TypeDyncfg),
	})
}
