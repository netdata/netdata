// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	frameworkfunctions "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"gopkg.in/yaml.v2"
)

const (
	SecretGraphClaim = "dyncfg:secretstores"
	dynCfgSecretPath = "/collectors/%s/SecretStores"
)

type ControllerConfig struct {
	Epoch        uint64                      // run generation this controller belongs to
	PluginName   string                      // owning plugin name
	Frames       *lifecycle.FrameOwner       // protocol frame sink
	Store        *secretstore.SecretStore    // per-run secret store
	Creators     *secretstore.CreatorCatalog // frozen creator catalog
	Dependencies *SecretDependencyIndex      // secret dependency index
	Initial      []secretstore.Config        // initial (stock/user) secret store configs
	Diagnostics  jobmgr.DiagnosticObserver   // operational log sink
}

type Controller struct {
	mu sync.Mutex // guards entries, published, restarts, and initial

	epoch        uint64                      // run generation
	prefix       string                      // "<plugin>:secretstore:" ID prefix
	path         string                      // "/collectors/<plugin>/SecretStores" config path
	frames       *lifecycle.FrameOwner       // protocol frame sink
	store        *secretstore.SecretStore    // per-run secret store
	creators     *secretstore.CreatorCatalog // frozen creator catalog
	dependencies *SecretDependencyIndex      // secret dependency index
	diagnostics  jobmgr.DiagnosticObserver   // operational log sink
	initial      []secretstore.Config        // initial secret store configs
	entries      map[string]secretEntry      // published store entries by key
	restarts     *SecretRestartCommand       // dependent-job restart command (bound at Bind)
	published    bool                        // initial snapshot has been published
}

type secretEntry struct {
	config secretstore.Config // the store's config
	status dyncfg.Status      // dyncfg status (running / failed)
}

func NewController(config ControllerConfig) (*Controller, error) {
	if config.Epoch == 0 ||
		config.PluginName == "" ||
		config.Frames == nil ||
		config.Store == nil ||
		config.Creators == nil ||
		config.Dependencies == nil {
		return nil, errors.New("jobmgr secrets: incomplete controller configuration")
	}
	return &Controller{
		epoch:        config.Epoch,
		prefix:       fmt.Sprintf("%s:secretstore:", config.PluginName),
		path:         fmt.Sprintf(dynCfgSecretPath, config.PluginName),
		frames:       config.Frames,
		store:        config.Store,
		creators:     config.Creators,
		dependencies: config.Dependencies,
		diagnostics:  config.Diagnostics,
		initial:      slices.Clone(config.Initial),
		entries:      make(map[string]secretEntry),
	}, nil
}

func (c *Controller) Bind(jobs DependentJobPort) error {
	if c == nil || jobs == nil {
		return errors.New("jobmgr secrets: invalid controller binding")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.restarts != nil || c.published {
		return errors.New("jobmgr secrets: duplicate or late controller binding")
	}
	restarts, err := NewSecretRestartCommand(c.epoch, c.dependencies, jobs, c.diagnostics)
	if err != nil {
		return err
	}
	c.restarts = restarts
	return nil
}

func (c *Controller) Prefix() string {
	if c == nil {
		return ""
	}
	return c.prefix
}

func (c *Controller) Prepare(
	ctx context.Context,
	input CommandInput,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if c == nil || ctx == nil || !scope.Valid() {
		return nil, errors.New("jobmgr secrets: invalid transaction preparation")
	}
	c.mu.Lock()
	published := c.published
	c.mu.Unlock()
	if !published {
		return c.noopMessageWithPermit(
			scope,
			current,
			permit,
			503,
			"Secretstore configuration is not published yet.",
		)
	}
	target, failure := c.resolveTarget(input)
	if failure != nil {
		transaction, err := c.noopMessageWithPermit(scope, current, permit, failure.code, failure.message)
		return c.observeTransaction(target, transaction, err)
	}
	var transaction lifecycle.PreparedResourceTransaction
	var err error
	switch target.command {
	case dyncfg.CommandSchema:
		transaction, err = c.prepareSchema(scope, current, target)
	case dyncfg.CommandGet:
		transaction, err = c.prepareGet(scope, current, target)
	case dyncfg.CommandUserconfig:
		transaction, err = c.prepareUserConfig(scope, current, input, target)
	case dyncfg.CommandTest:
		transaction, err = c.prepareTest(ctx, scope, current, input, target)
	case dyncfg.CommandAdd:
		transaction, err = c.prepareAdd(ctx, scope, current, permit, input, target)
	case dyncfg.CommandUpdate:
		transaction, err = c.prepareUpdate(ctx, scope, current, permit, input, target)
	case dyncfg.CommandRemove:
		transaction, err = c.prepareRemove(scope, current, target)
	default:
		transaction, err = c.noopMessageWithPermit(
			scope,
			current,
			permit,
			501,
			fmt.Sprintf("Function '%s' command '%s' is not implemented.", "config", target.command),
		)
	}
	return c.observeTransaction(target, transaction, err)
}

type secretTarget struct {
	command dyncfg.Command        // parsed dyncfg command
	id      string                // raw config id
	key     string                // store key (kind_name)
	kind    secretstore.StoreKind // store kind
	name    string                // store name
}

type targetFailure struct {
	code    int
	message string
}

func (c *Controller) resolveTarget(input CommandInput) (secretTarget, *targetFailure) {
	if len(input.Args) < 2 {
		return secretTarget{}, &targetFailure{
			code:    400,
			message: fmt.Sprintf("missing required arguments: need 2, got %d", len(input.Args)),
		}
	}
	id := input.Args[0]
	rest, ok := strings.CutPrefix(id, c.prefix)
	if !ok || rest == "" {
		return secretTarget{}, &targetFailure{
			code:    400,
			message: "invalid config ID format.",
		}
	}
	command := dyncfg.CommandFromArgs(input.Args)
	target := secretTarget{
		command: command,
		id:      id,
	}
	if command == dyncfg.CommandAdd {
		if len(input.Args) < 3 {
			return secretTarget{}, &targetFailure{
				code:    400,
				message: fmt.Sprintf("missing required arguments: need 3, got %d", len(input.Args)),
			}
		}
		if strings.Contains(rest, ":") {
			return secretTarget{}, &targetFailure{
				code:    400,
				message: "invalid secretstore template ID.",
			}
		}
		target.kind = secretstore.StoreKind(rest)
		target.name = input.Args[2]
		if _, ok := c.creators.Lookup(target.kind); !ok {
			return secretTarget{}, &targetFailure{
				code:    404,
				message: fmt.Sprintf("The specified secretstore kind '%s' is not supported.", target.kind),
			}
		}
		if err := dyncfg.JobNameRuleAllowDots(target.name); err != nil {
			return secretTarget{}, &targetFailure{
				code:    400,
				message: fmt.Sprintf("invalid config name '%s': %v.", target.name, err),
			}
		}
		target.key = secretstore.StoreKey(target.kind, target.name)
		return target, nil
	}
	if !strings.Contains(rest, ":") {
		kind := secretstore.StoreKind(rest)
		if command == dyncfg.CommandSchema || command == dyncfg.CommandUserconfig {
			if _, ok := c.creators.Lookup(kind); ok {
				target.kind = kind
				return target, nil
			}
		}
		return secretTarget{}, &targetFailure{
			code:    400,
			message: "invalid config ID format.",
		}
	}
	kind, name, err := secretstore.ParseStoreKey(rest)
	if err != nil {
		return secretTarget{}, &targetFailure{
			code:    400,
			message: "invalid config ID format.",
		}
	}
	target.key, target.kind, target.name = secretstore.StoreKey(kind, name), kind, name
	return target, nil
}

func (c *Controller) entry(key string) (secretEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if ok {
		entry.config, _ = c.store.Config(key)
		if entry.config == nil {
			entry.config = cloneSecretConfig(c.entries[key].config)
		}
	}
	return entry, ok
}

func (c *Controller) commitEntry(key string, entry *secretEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry == nil {
		delete(c.entries, key)
		return
	}
	c.entries[key] = secretEntry{
		config: cloneSecretConfig(entry.config),
		status: entry.status,
	}
}

func cloneSecretConfig(config secretstore.Config) secretstore.Config {
	if config == nil {
		return nil
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		return nil
	}
	var cloned secretstore.Config
	if yaml.Unmarshal(payload, &cloned) != nil {
		return nil
	}
	return cloned
}

func mustSecretMessage(code int, message string) lifecycle.SealedResult {
	result, err := lifecycle.NewSealedResult(
		code,
		"application/json",
		frameworkfunctions.BuildJSONPayload(code, message),
	)
	if err != nil {
		panic(err)
	}
	return result
}

func (c *Controller) protocolCleanup(build func(*netdataapi.API)) lifecycle.TaskCleanup {
	if c == nil || build == nil {
		return func() error {
			return errors.New("jobmgr secrets: invalid protocol cleanup")
		}
	}
	var payload bytes.Buffer
	build(netdataapi.New(&payload))
	prepared, err := lifecycle.PrepareProtocolFrame(payload.Bytes())
	if err != nil {
		return func() error { return err }
	}
	return func() error {
		return c.frames.CommitPreparedProtocolFrame(prepared)
	}
}

func (c *Controller) configCreateCleanup(entry secretEntry) lifecycle.TaskCleanup {
	commands := dyncfg.JoinCommands(
		dyncfg.CommandSchema,
		dyncfg.CommandGet,
		dyncfg.CommandUpdate,
		dyncfg.CommandTest,
		dyncfg.CommandUserconfig,
	)
	if entry.config.SourceType() == confgroup.TypeDyncfg {
		commands += " " + string(dyncfg.CommandRemove)
	}
	return c.protocolCleanup(func(api *netdataapi.API) {
		api.CONFIGCREATE(netdataapi.ConfigOpts{
			ID:                c.prefix + entry.config.ExposedKey(),
			Status:            entry.status.String(),
			ConfigType:        dyncfg.ConfigTypeJob.String(),
			Path:              c.path,
			SourceType:        entry.config.SourceType(),
			Source:            entry.config.Source(),
			SupportedCommands: commands,
		})
	})
}

func (c *Controller) configDeleteCleanup(key string) lifecycle.TaskCleanup {
	return c.protocolCleanup(func(api *netdataapi.API) {
		api.CONFIGDELETE(c.prefix + key)
	})
}

func (c *Controller) templateCleanup() lifecycle.TaskCleanup {
	kinds := c.creators.Kinds()
	if len(kinds) == 0 {
		return func() error { return nil }
	}
	return c.protocolCleanup(func(api *netdataapi.API) {
		for _, kind := range kinds {
			api.CONFIGCREATE(netdataapi.ConfigOpts{
				ID:         c.prefix + string(kind),
				Status:     dyncfg.StatusAccepted.String(),
				ConfigType: dyncfg.ConfigTypeTemplate.String(),
				Path:       c.path,
				SourceType: "internal",
				Source:     "internal",
				SupportedCommands: dyncfg.JoinCommands(
					dyncfg.CommandAdd,
					dyncfg.CommandSchema,
					dyncfg.CommandUserconfig,
				),
			})
		}
	})
}

func parseSecretPayload(input CommandInput, target any) error {
	if !input.HasPayload || len(input.Payload) == 0 {
		return errors.New("payload is required")
	}
	if strings.Contains(strings.ToLower(input.ContentType), "json") {
		return json.Unmarshal(input.Payload, target)
	}
	return yaml.Unmarshal(input.Payload, target)
}

func typedSecretConfig(creators *secretstore.CreatorCatalog, kind secretstore.StoreKind) (any, error) {
	store, ok := creators.New(kind)
	if !ok {
		return nil, fmt.Errorf("the specified secretstore kind '%s' is not supported", kind)
	}
	config := store.Configuration()
	if config == nil {
		return nil, fmt.Errorf("secretstore kind '%s' does not provide configuration", kind)
	}
	return config, nil
}

func secretResourceID(key string) string {
	kind, name, err := secretstore.ParseStoreKey(key)
	if err != nil {
		return ""
	}
	return "secretstore:" + string(kind) + "_" + name
}

func sortedInitialConfigs(configs []secretstore.Config) []secretstore.Config {
	cloned := slices.Clone(configs)
	slices.SortStableFunc(cloned, func(a, b secretstore.Config) int {
		if a.ExposedKey() != b.ExposedKey() {
			return cmp.Compare(a.ExposedKey(), b.ExposedKey())
		}
		return cmp.Compare(b.SourceTypePriority(), a.SourceTypePriority())
	})
	return cloned
}
