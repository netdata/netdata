// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const (
	dyncfgSecretStorePrefixf = "%s:secretstore:"
	dyncfgSecretStorePath    = "/collectors/%s/SecretStores"
)

type Options struct {
	Logger  *logger.Logger
	API     *dyncfg.Responder
	Seen    *dyncfg.SeenCache[secretstore.Config]
	Exposed *dyncfg.ExposedCache[secretstore.Config]
	Service secretstore.Service
	Plugin  string
	Initial []secretstore.Config

	AffectedJobs            func(string) []secretstore.JobRef
	RestartableAffectedJobs func(string) []secretstore.JobRef
	RestartDependentJobs    func(string) string
}

type Entry struct {
	Cfg    secretstore.Config
	Status dyncfg.Status
}

type Controller struct {
	*logger.Logger

	api        *dyncfg.Responder
	seen       *dyncfg.SeenCache[secretstore.Config]
	exposed    *dyncfg.ExposedCache[secretstore.Config]
	service    secretstore.Service
	pluginName string
	initial    []secretstore.Config

	affectedJobs            func(string) []secretstore.JobRef
	restartableAffectedJobs func(string) []secretstore.JobRef
	restartDependentJobs    func(string) string

	handler *dyncfg.Handler[secretstore.Config]
	cb      *secretStoreCallbacks
}

func New(opts Options) *Controller {
	seen := opts.Seen
	if seen == nil {
		seen = dyncfg.NewSeenCache[secretstore.Config]()
	}
	exposed := opts.Exposed
	if exposed == nil {
		exposed = dyncfg.NewExposedCache[secretstore.Config]()
	}

	c := &Controller{
		Logger:                  opts.Logger,
		api:                     opts.API,
		seen:                    seen,
		exposed:                 exposed,
		service:                 opts.Service,
		pluginName:              opts.Plugin,
		initial:                 append([]secretstore.Config(nil), opts.Initial...),
		affectedJobs:            opts.AffectedJobs,
		restartableAffectedJobs: opts.RestartableAffectedJobs,
		restartDependentJobs:    opts.RestartDependentJobs,
	}
	c.cb = newSecretStoreCallbacks(secretStoreCallbackDeps{
		pluginName:           c.pluginName,
		log:                  c.Logger,
		service:              c.service,
		restartDependentJobs: c.restartDependentJobs,
	})
	c.handler = dyncfg.NewHandler(dyncfg.HandlerOpts[secretstore.Config]{
		Logger:    c.Logger,
		API:       c.api,
		Seen:      c.seen,
		Exposed:   c.exposed,
		Callbacks: c.cb,
		Path:      fmt.Sprintf(dyncfgSecretStorePath, c.pluginName),
		JobCommands: []dyncfg.Command{
			dyncfg.CommandSchema,
			dyncfg.CommandGet,
			dyncfg.CommandUpdate,
			dyncfg.CommandTest,
			dyncfg.CommandUserconfig,
		},
	})
	return c
}

func (c *Controller) Prefix() string {
	return fmt.Sprintf(dyncfgSecretStorePrefixf, c.pluginName)
}

func (c *Controller) CreateTemplates() {
	if c.service == nil || c.api == nil {
		return
	}
	for _, kind := range c.service.Kinds() {
		c.api.ConfigCreate(netdataapi.ConfigOpts{
			ID:                c.templateID(kind),
			Status:            dyncfg.StatusAccepted.String(),
			ConfigType:        dyncfg.ConfigTypeTemplate.String(),
			Path:              fmt.Sprintf(dyncfgSecretStorePath, c.pluginName),
			SourceType:        "internal",
			Source:            "internal",
			SupportedCommands: dyncfgSecretStoreTemplateCmds(),
		})
	}
}

func (c *Controller) PublishExisting() {
	if len(c.initial) != 0 {
		for _, cfg := range c.initial {
			c.publishInitialConfig(cfg)
		}
		c.initial = nil
		return
	}

	c.exposed.ForEach(func(_ string, entry *dyncfg.Entry[secretstore.Config]) bool {
		c.handler.NotifyJobCreate(entry.Cfg, entry.Status)
		return true
	})
}

func (c *Controller) SetAPI(api *dyncfg.Responder) {
	if api == nil {
		// Nil means "keep the current responder" rather than clearing output wiring.
		return
	}
	c.api = api
	if c.handler != nil {
		c.handler.SetAPI(api)
	}
}

func (c *Controller) Service() secretstore.Service {
	return c.service
}

func (c *Controller) configID(id string) string {
	return fmt.Sprintf("%s%s", c.Prefix(), id)
}

func (c *Controller) templateID(kind secretstore.StoreKind) string {
	return fmt.Sprintf("%s%s", c.Prefix(), kind)
}

func entryFromDyncfg(entry *dyncfg.Entry[secretstore.Config]) Entry {
	if entry == nil {
		return Entry{}
	}
	return Entry{Cfg: entry.Cfg, Status: entry.Status}
}
