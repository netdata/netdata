// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

const (
	// A rendered response is capped at 4096 bytes. Each job-name list gets a
	// 3584-byte content budget, reserving 512 bytes for surrounding prose and its
	// truncation marker; boundSecretMessage enforces the total after lists combine.
	maximumSecretJobSummaryBytes = 4 * 1024
	secretJobSummaryContentBytes = 7 * 512
)

func (c *Controller) prepareSchema(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	if target.key != "" {
		if _, ok := c.entry(target.key); !ok {
			return c.noopMessage(
				scope,
				current,
				404,
				fmt.Sprintf(
					"The specified secretstore '%s' is not configured.",
					target.key,
				),
			)
		}
	}
	schema, ok := c.creators.Schema(target.kind)
	if !ok {
		return c.noopMessage(
			scope,
			current,
			404,
			fmt.Sprintf(
				"The specified secretstore kind '%s' is not supported.",
				target.kind,
			),
		)
	}
	result, err := lifecycle.NewSealedResult(
		200,
		"application/json",
		[]byte(schema),
	)
	if err != nil {
		return nil, err
	}
	return c.noop(
		scope,
		current,
		lifecycle.LongLivedPermit{},
		result,
		nil,
		nil,
	)
}

func (c *Controller) prepareGet(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, ok := c.entry(target.key)
	if !ok {
		return c.noopMessage(
			scope,
			current,
			404,
			fmt.Sprintf(
				"The specified secretstore '%s' is not configured.",
				target.key,
			),
		)
	}
	typed, err := typedSecretConfig(
		c.creators,
		entry.config.Kind(),
	)
	if err == nil {
		var payload []byte
		payload, err = yaml.Marshal(entry.config)
		if err == nil {
			err = yaml.Unmarshal(payload, typed)
		}
	}
	if err != nil {
		return c.noopMessage(
			scope,
			current,
			500,
			"Failed to materialize secretstore configuration.",
		)
	}
	payload, err := json.Marshal(typed)
	if err != nil {
		return nil, err
	}
	result, err := lifecycle.NewSealedResult(
		200,
		"application/json",
		payload,
	)
	if err != nil {
		return nil, err
	}
	return c.noop(
		scope,
		current,
		lifecycle.LongLivedPermit{},
		result,
		nil,
		nil,
	)
}

func (c *Controller) prepareUserConfig(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	typed, err := typedSecretConfig(
		c.creators,
		target.kind,
	)
	if err == nil {
		err = parseSecretPayload(input, typed)
	}
	if err != nil {
		return c.noopMessage(
			scope,
			current,
			400,
			"Invalid secretstore configuration.",
		)
	}
	payload, err := yaml.Marshal(typed)
	if err != nil {
		return nil, err
	}
	result, err := lifecycle.NewSealedResult(
		200,
		"application/yaml",
		payload,
	)
	if err != nil {
		return nil, err
	}
	return c.noop(
		scope,
		current,
		lifecycle.LongLivedPermit{},
		result,
		nil,
		nil,
	)
}

func (c *Controller) prepareTest(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, ok := c.entry(target.key)
	if !ok {
		return c.noopMessage(
			scope,
			current,
			404,
			fmt.Sprintf(
				"The specified secretstore '%s' is not configured.",
				target.key,
			),
		)
	}
	config := entry.config
	validationOnly := true
	if input.HasPayload {
		var err error
		config, err = c.configFromPayload(input, target)
		if err != nil {
			return c.noopMessage(
				scope,
				current,
				400,
				"Invalid secretstore configuration.",
			)
		}
		validationOnly = false
	}
	if err := c.store.Validate(
		ctx,
		c.creators,
		config,
	); err != nil {
		return c.noopMessage(
			scope,
			current,
			400,
			"Secretstore configuration validation failed.",
		)
	}
	if !validationOnly && config.Hash() == entry.config.Hash() {
		return c.noopMessage(
			scope,
			current,
			202,
			"Submitted configuration does not change the active secretstore.",
		)
	}
	affected := formatSecretJobs(
		c.dependencies.Affected(target.key, false),
	)
	restartable := formatSecretJobs(
		c.dependencies.Affected(target.key, true),
	)
	return c.noopMessage(
		scope,
		current,
		202,
		secretImpactMessage(
			affected,
			restartable,
			validationOnly,
		),
	)
}

func (c *Controller) prepareAdd(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	if current != nil || scope.Current.Valid() {
		return c.noop(
			scope,
			current,
			permit,
			mustSecretMessage(
				409,
				fmt.Sprintf(
					"The specified secretstore '%s' already exists.",
					target.key,
				),
			),
			nil,
			nil,
		)
	}
	if _, exists := c.entry(target.key); exists {
		return c.noop(
			scope,
			current,
			permit,
			mustSecretMessage(
				409,
				fmt.Sprintf(
					"The specified secretstore '%s' already exists.",
					target.key,
				),
			),
			nil,
			nil,
		)
	}
	config, err := c.configFromPayload(input, target)
	if err != nil {
		return c.noop(
			scope,
			current,
			permit,
			mustSecretMessage(
				400,
				"Invalid secretstore configuration.",
			),
			nil,
			nil,
		)
	}
	return c.prepareStoreMutation(
		ctx,
		scope,
		current,
		permit,
		config,
		0,
		true,
	)
}

func (c *Controller) prepareUpdate(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, exists := c.entry(target.key)
	if !exists {
		return c.noop(
			scope,
			current,
			permit,
			mustSecretMessage(
				404,
				fmt.Sprintf(
					"The specified secretstore '%s' is not configured.",
					target.key,
				),
			),
			nil,
			nil,
		)
	}
	config, err := c.configFromPayload(input, target)
	if err != nil {
		return c.noop(
			scope,
			current,
			permit,
			mustSecretMessage(
				400,
				"Invalid secretstore configuration.",
			),
			nil,
			nil,
		)
	}
	config.SetSource(confgroup.TypeDyncfg)
	config.SetSourceType(confgroup.TypeDyncfg)
	expected := c.store.Generation(target.key)
	if expected != 0 && entry.config.Hash() == config.Hash() {
		return c.noop(
			scope,
			current,
			permit,
			mustSecretMessage(200, ""),
			nil,
			c.configCreateCleanup(entry),
		)
	}
	return c.prepareStoreMutation(
		ctx,
		scope,
		current,
		permit,
		config,
		expected,
		false,
	)
}

func (c *Controller) prepareRemove(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, exists := c.entry(target.key)
	if !exists {
		return c.noopMessage(
			scope,
			current,
			404,
			fmt.Sprintf(
				"The specified secretstore '%s' is not configured.",
				target.key,
			),
		)
	}
	if affected := formatSecretJobs(c.dependencies.Affected(target.key, false)); affected != "" {
		return c.noopMessage(
			scope,
			current,
			409,
			fmt.Sprintf(
				"The specified secretstore '%s' is used by jobs (%s).",
				target.key,
				affected,
			),
		)
	}
	if entry.config.SourceType() != confgroup.TypeDyncfg {
		return c.noopMessage(
			scope,
			current,
			405,
			fmt.Sprintf(
				"removing configurations of source type '%s' is not supported, only 'dyncfg' configurations can be removed.",
				entry.config.SourceType(),
			),
		)
	}
	expected := c.store.Generation(target.key)
	if expected == 0 || current == nil || !scope.Current.Valid() {
		return c.noopMessage(
			scope,
			current,
			409,
			"Secretstore has no active generation.",
		)
	}
	mutation, err := c.store.PrepareRemoval(
		target.key,
		expected,
	)
	if err != nil {
		return nil, err
	}
	return newPreparedSecretTransaction(
		preparedSecretSpec{
			scope:       scope,
			current:     current,
			store:       c.store,
			storeKey:    target.key,
			mutation:    mutation,
			remove:      true,
			result:      mustSecretMessage(200, ""),
			cleanup:     c.configDeleteCleanup(target.key),
			controller:  c,
			removeEntry: true,
		},
	)
}

func (c *Controller) configFromPayload(
	input CommandInput,
	target secretTarget,
) (secretstore.Config, error) {
	var config secretstore.Config
	if err := parseSecretPayload(input, &config); err != nil {
		return nil, err
	}
	if config == nil {
		config = secretstore.Config{}
	}
	config.SetName(target.name)
	config.SetKind(target.kind)
	config.SetSource(confgroup.TypeDyncfg)
	config.SetSourceType(confgroup.TypeDyncfg)
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func (c *Controller) prepareStoreMutation(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	config secretstore.Config,
	expected uint64,
	installFailure bool,
) (lifecycle.PreparedResourceTransaction, error) {
	if !permit.Valid() ||
		!scope.Successor.Valid() ||
		permit.Owner() != scope.Successor {
		return nil, errors.New(
			"jobmgr secrets: invalid Store mutation permit",
		)
	}
	carrier, err := newStoreGenerationCarrier(
		permit,
		scope.Successor,
	)
	if err != nil {
		return nil, err
	}
	mutation, prepareErr := c.store.PrepareMutation(
		ctx,
		c.creators,
		carrier,
		config,
		expected,
	)
	entry := secretEntry{
		config: config, status: dyncfg.StatusRunning,
	}
	if prepareErr != nil {
		if mutation.Valid() {
			if installFailure {
				entry.status = dyncfg.StatusFailed
			}
			cleanup := func() error { return nil }
			var storedEntry *secretEntry
			if installFailure {
				cleanup = c.configCreateCleanup(entry)
				storedEntry = &entry
			}
			return newPreparedSecretTransaction(
				preparedSecretSpec{
					scope: scope, current: current,
					permit:   permit,
					store:    c.store,
					storeKey: config.ExposedKey(),
					mutation: mutation,
					abort:    true,
					result: mustSecretMessage(
						400,
						"Secretstore configuration validation failed.",
					),
					cleanup:    cleanup,
					controller: c,
					entry:      storedEntry,
				},
			)
		}
		if !installFailure {
			return newPreparedSecretTransaction(
				preparedSecretSpec{
					scope: scope, current: current,
					permit:   permit,
					store:    c.store,
					storeKey: config.ExposedKey(),
					result: mustSecretMessage(
						400,
						"Secretstore configuration validation failed.",
					),
					cleanup:    func() error { return nil },
					controller: c,
				},
			)
		}
		entry.status = dyncfg.StatusFailed
		return newPreparedSecretTransaction(
			preparedSecretSpec{
				scope: scope, current: current,
				permit:   permit,
				store:    c.store,
				storeKey: config.ExposedKey(),
				result: mustSecretMessage(
					400,
					"Secretstore configuration validation failed.",
				),
				cleanup:    c.configCreateCleanup(entry),
				controller: c, entry: &entry,
			},
		)
	}
	return newPreparedSecretTransaction(
		preparedSecretSpec{
			scope: scope, current: current,
			permit:     permit,
			store:      c.store,
			storeKey:   config.ExposedKey(),
			mutation:   mutation,
			result:     mustSecretMessage(200, ""),
			cleanup:    c.configCreateCleanup(entry),
			controller: c, entry: &entry,
			restarts: c.restartCommand(),
		},
	)
}

func (c *Controller) restartCommand() *SecretRestartCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.restarts
}

func (c *Controller) noop(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	result lifecycle.SealedResult,
	entry *secretEntry,
	cleanup lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	if cleanup == nil {
		cleanup = func() error { return nil }
	}
	return newPreparedSecretTransaction(
		preparedSecretSpec{
			scope: scope, current: current,
			permit: permit, result: result, cleanup: cleanup,
			controller: c, entry: entry,
		},
	)
}

func (c *Controller) noopMessage(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	code int,
	message string,
) (lifecycle.PreparedResourceTransaction, error) {
	return c.noop(
		scope,
		current,
		lifecycle.LongLivedPermit{},
		mustSecretMessage(code, message),
		nil,
		nil,
	)
}

func formatSecretJobs(refs []secretstore.JobRef) string {
	return formatBoundedSecretNames(
		len(refs),
		func(index int) string {
			ref := refs[index]
			if ref.Display != "" {
				return ref.Display
			}
			return ref.ID
		},
	)
}

func formatSecretJobNames(names []string) string {
	return formatBoundedSecretNames(
		len(names),
		func(index int) string {
			return names[index]
		},
	)
}

func formatBoundedSecretNames(
	count int,
	name func(int) string,
) string {
	if count <= 0 || name == nil {
		return ""
	}
	var builder strings.Builder
	builder.Grow(maximumSecretJobSummaryBytes)
	for index := range count {
		value := name(index)
		separatorBytes := 0
		if builder.Len() != 0 {
			separatorBytes = 2
		}
		if len(value) >
			secretJobSummaryContentBytes-
				builder.Len()-
				separatorBytes {
			if builder.Len() != 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("... and ")
			builder.WriteString(strconv.Itoa(count - index))
			builder.WriteString(" more")
			break
		}
		if separatorBytes != 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(value)
	}
	return builder.String()
}

func secretImpactMessage(
	affected string,
	restartable string,
	validationOnly bool,
) string {
	var message string
	if validationOnly {
		if affected == "" {
			message = "Stored configuration is valid. No jobs are currently using this secretstore."
		} else if restartable == "" {
			message = "Stored configuration is valid. This secretstore is used by jobs: " +
				affected +
				". No running jobs would be restarted automatically by a change."
		} else {
			message = "Stored configuration is valid. This secretstore is used by jobs: " +
				affected +
				". Running jobs that would be restarted automatically by a change: " +
				restartable + "."
		}
	} else if affected == "" {
		message = "No jobs currently use this secretstore."
	} else if restartable == "" {
		message = "Updated configuration is used by jobs: " +
			affected +
			". No running jobs would be restarted automatically."
	} else {
		message = "Updated configuration is used by jobs: " +
			affected +
			". Running jobs that would be restarted automatically: " +
			restartable + "."
	}
	return boundSecretMessage(message)
}

func boundSecretMessage(message string) string {
	if len(message) <= maximumSecretJobSummaryBytes {
		return message
	}
	const suffix = "... [truncated]"
	end := maximumSecretJobSummaryBytes - len(suffix)
	for end > 0 && !utf8.RuneStart(message[end]) {
		end--
	}
	return message[:end] + suffix
}
