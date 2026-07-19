// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

const (
	maximumSecretJobSummaryBytes = 4 * 1024
	secretJobSummaryContentBytes = 7 * 512
)

func (controller *Controller) prepareSchema(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	if target.key != "" {
		if _, ok := controller.entry(target.key); !ok {
			return controller.noop(
				scope,
				current,
				lifecycle.LongLivedPermit{},
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
	}
	schema, ok := controller.creators.Schema(target.kind)
	if !ok {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustSecretMessage(
				404,
				fmt.Sprintf(
					"The specified secretstore kind '%s' is not supported.",
					target.kind,
				),
			),
			nil,
			nil,
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
	return controller.noop(
		scope,
		current,
		lifecycle.LongLivedPermit{},
		result,
		nil,
		nil,
	)
}

func (controller *Controller) prepareGet(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, ok := controller.entry(target.key)
	if !ok {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
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
	typed, err := typedSecretConfig(
		controller.creators,
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
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustSecretMessage(
				500,
				"Failed to materialize secretstore configuration.",
			),
			nil,
			nil,
		)
	}
	payload, err := jsonMarshal(typed)
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
	return controller.noop(
		scope,
		current,
		lifecycle.LongLivedPermit{},
		result,
		nil,
		nil,
	)
}

func (controller *Controller) prepareUserConfig(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	typed, err := typedSecretConfig(
		controller.creators,
		target.kind,
	)
	if err == nil {
		err = parseSecretPayload(input, typed)
	}
	if err != nil {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustSecretMessage(
				400,
				"Invalid secretstore configuration.",
			),
			nil,
			nil,
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
	return controller.noop(
		scope,
		current,
		lifecycle.LongLivedPermit{},
		result,
		nil,
		nil,
	)
}

func (controller *Controller) prepareTest(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, ok := controller.entry(target.key)
	if !ok {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
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
	config := entry.config
	validationOnly := true
	if input.HasPayload {
		var err error
		config, err = controller.configFromPayload(input, target)
		if err != nil {
			return controller.noop(
				scope,
				current,
				lifecycle.LongLivedPermit{},
				mustSecretMessage(
					400,
					"Invalid secretstore configuration.",
				),
				nil,
				nil,
			)
		}
		validationOnly = false
	}
	if err := controller.store.Validate(
		ctx,
		controller.creators,
		config,
	); err != nil {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustSecretMessage(
				400,
				"Secretstore configuration validation failed.",
			),
			nil,
			nil,
		)
	}
	if !validationOnly && config.Hash() == entry.config.Hash() {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustSecretMessage(
				202,
				"Submitted configuration does not change the active secretstore.",
			),
			nil,
			nil,
		)
	}
	affected := formatSecretJobs(
		controller.dependencies.Affected(target.key, false),
	)
	restartable := formatSecretJobs(
		controller.dependencies.Affected(target.key, true),
	)
	return controller.noop(
		scope,
		current,
		lifecycle.LongLivedPermit{},
		mustSecretMessage(
			202,
			secretImpactMessage(
				affected,
				restartable,
				validationOnly,
			),
		),
		nil,
		nil,
	)
}

func (controller *Controller) prepareAdd(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	if current != nil || scope.Current.Valid() {
		return controller.noop(
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
	if _, exists := controller.entry(target.key); exists {
		return controller.noop(
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
	config, err := controller.configFromPayload(input, target)
	if err != nil {
		return controller.noop(
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
	return controller.prepareStoreMutation(
		ctx,
		scope,
		current,
		permit,
		config,
		0,
		true,
	)
}

func (controller *Controller) prepareUpdate(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, exists := controller.entry(target.key)
	if !exists {
		return controller.noop(
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
	config, err := controller.configFromPayload(input, target)
	if err != nil {
		return controller.noop(
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
	expected := controller.store.Generation(target.key)
	if expected != 0 && entry.config.Hash() == config.Hash() {
		return controller.noop(
			scope,
			current,
			permit,
			mustSecretMessage(200, ""),
			nil,
			controller.configCreateCleanup(entry),
		)
	}
	return controller.prepareStoreMutation(
		ctx,
		scope,
		current,
		permit,
		config,
		expected,
		false,
	)
}

func (controller *Controller) prepareRemove(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, exists := controller.entry(target.key)
	if !exists {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
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
	if affected := formatSecretJobs(
		controller.dependencies.Affected(target.key, false),
	); affected != "" {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustSecretMessage(
				409,
				fmt.Sprintf(
					"The specified secretstore '%s' is used by jobs (%s).",
					target.key,
					affected,
				),
			),
			nil,
			nil,
		)
	}
	if entry.config.SourceType() != confgroup.TypeDyncfg {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustSecretMessage(
				405,
				fmt.Sprintf(
					"removing configurations of source type '%s' is not supported, only 'dyncfg' configurations can be removed.",
					entry.config.SourceType(),
				),
			),
			nil,
			nil,
		)
	}
	expected := controller.store.Generation(target.key)
	if expected == 0 || current == nil || !scope.Current.Valid() {
		return controller.noop(
			scope,
			current,
			lifecycle.LongLivedPermit{},
			mustSecretMessage(
				409,
				"Secretstore has no active generation.",
			),
			nil,
			nil,
		)
	}
	mutation, err := controller.store.PrepareRemoval(
		target.key,
		expected,
	)
	if err != nil {
		return nil, err
	}
	return newPreparedSecretTransaction(
		preparedSecretSpec{
			scope: scope, current: current,
			store: controller.store, storeKey: target.key,
			mutation: &mutation, remove: true,
			result:     mustSecretMessage(200, ""),
			cleanup:    controller.configDeleteCleanup(target.key),
			controller: controller, removeEntry: true,
		},
	)
}

func (controller *Controller) configFromPayload(
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

func (controller *Controller) prepareStoreMutation(
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
	mutation, prepareErr := controller.store.PrepareMutation(
		ctx,
		controller.creators,
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
				cleanup = controller.configCreateCleanup(entry)
				storedEntry = &entry
			}
			return newPreparedSecretTransaction(
				preparedSecretSpec{
					scope: scope, current: current,
					permit:   permit,
					store:    controller.store,
					storeKey: config.ExposedKey(),
					mutation: &mutation,
					abort:    true,
					result: mustSecretMessage(
						400,
						"Secretstore configuration validation failed.",
					),
					cleanup:    cleanup,
					controller: controller,
					entry:      storedEntry,
				},
			)
		}
		if !installFailure {
			return newPreparedSecretTransaction(
				preparedSecretSpec{
					scope: scope, current: current,
					permit:   permit,
					store:    controller.store,
					storeKey: config.ExposedKey(),
					result: mustSecretMessage(
						400,
						"Secretstore configuration validation failed.",
					),
					cleanup:    func() error { return nil },
					controller: controller,
				},
			)
		}
		entry.status = dyncfg.StatusFailed
		return newPreparedSecretTransaction(
			preparedSecretSpec{
				scope: scope, current: current,
				permit:   permit,
				store:    controller.store,
				storeKey: config.ExposedKey(),
				result: mustSecretMessage(
					400,
					"Secretstore configuration validation failed.",
				),
				cleanup:    controller.configCreateCleanup(entry),
				controller: controller, entry: &entry,
			},
		)
	}
	return newPreparedSecretTransaction(
		preparedSecretSpec{
			scope: scope, current: current,
			permit:     permit,
			store:      controller.store,
			storeKey:   config.ExposedKey(),
			mutation:   &mutation,
			result:     mustSecretMessage(200, ""),
			cleanup:    controller.configCreateCleanup(entry),
			controller: controller, entry: &entry,
			restarts: controller.restartCommand(),
		},
	)
}

func (controller *Controller) restartCommand() *SecretRestartCommand {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return controller.restarts
}

func (controller *Controller) noop(
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
			controller: controller, entry: entry,
		},
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
	for index := 0; index < count; index++ {
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
			fmt.Fprintf(
				&builder,
				"... and %d more",
				count-index,
			)
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

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}
