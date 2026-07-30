// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/agent/internal/naming"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type materializationResult[T any] struct {
	value T
	err   error
}

type materializationError struct {
	cause    error
	identity jobmgr.ProcessAttemptIdentity
}

type resourceErrorPhase uint8

const (
	resourceErrorConfiguration resourceErrorPhase = iota + 1
	resourceErrorConstruction
	resourceErrorOperationalTest
)

type resourceError struct {
	cause error
	phase resourceErrorPhase
}

func newResourceError(phase resourceErrorPhase, cause error) error {
	if cause == nil {
		return nil
	}
	return &resourceError{cause: cause, phase: phase}
}

func (err *resourceError) Error() string {
	// Resource errors may contain credential-bearing configuration.
	var message string
	switch {
	case err == nil:
		return "service discovery resource failed"
	case err.phase == resourceErrorConfiguration:
		message = "service discovery resource configuration is invalid"
	case err.phase == resourceErrorOperationalTest:
		message = "service discovery operational test failed"
	default:
		message = "service discovery resource construction failed"
	}
	if detail, ok := dyncfg.PublicMessage(err.cause); ok {
		return message + ": " + detail
	}
	return message
}

func (err *resourceError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *resourceError) dyncfgTestCode() int {
	if err != nil && err.phase == resourceErrorOperationalTest {
		return 422
	}
	return 400
}

func (err *materializationError) Error() string {
	if err != nil && errors.Is(err.cause, jobmgr.ErrProcessAttemptQuarantined) {
		return "service discovery configuration is unavailable until the plugin restarts"
	}
	return "service discovery configuration is busy; retry the command"
}

func (err *materializationError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (*materializationError) DyncfgCode() int {
	return 503
}

func runMaterialization[T any](
	ctx context.Context,
	discovery *ServiceDiscovery,
	key string,
	supersede bool,
	work func(context.Context) (T, error),
) (T, error) {
	var zero T
	if ctx == nil || discovery == nil || discovery.epoch == 0 ||
		discovery.attempts == nil || key == "" || work == nil {
		return zero, errors.New("service discovery: invalid materialization")
	}
	if cause := context.Cause(ctx); cause != nil {
		return zero, cause
	}
	identity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptServiceDiscovery,
		Key:       key,
		Resource:  "service discovery configuration",
	}
	if supersede {
		if err := discovery.attempts.SupersedeProcessAttempt(ctx, identity); err != nil {
			return zero, classifyMaterializationError(err, identity)
		}
	}
	resultCh := make(chan materializationResult[T], 1)
	attempt, err := discovery.attempts.StartProcessAttempt(ctx, jobmgr.ProcessAttemptPlan{
		Identity: identity,
		Target:   discovery.epoch,
		Work: func(
			attemptCtx context.Context,
			_ jobmgr.ProcessAttemptAdmission,
		) error {
			value, workErr := work(attemptCtx)
			resultCh <- materializationResult[T]{value: value, err: workErr}
			return workErr
		},
	})
	if err != nil {
		return zero, classifyMaterializationError(err, identity)
	}
	if err := attempt.Await(ctx); err != nil {
		return zero, classifyMaterializationError(err, identity)
	}
	select {
	case result := <-resultCh:
		return result.value, result.err
	default:
		return zero, errors.New("service discovery: materialization settled without a result")
	}
}

func classifyMaterializationError(
	err error,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	if errors.Is(err, jobmgr.ErrProcessAttemptBusy) ||
		errors.Is(err, jobmgr.ErrProcessAttemptDeadline) ||
		errors.Is(err, jobmgr.ErrProcessAttemptSuperseded) ||
		errors.Is(err, jobmgr.ErrProcessAttemptQuarantined) {
		return &materializationError{cause: err, identity: identity}
	}
	return err
}

func materializationIdentity(prefix string, values ...[]byte) string {
	hash := sha256.New()
	var length [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(value)
	}
	return prefix + "\x00" + string(hash.Sum(nil))
}

func (d *ServiceDiscovery) prepareDyncfgConfig(
	fn dyncfg.Function,
	name string,
) (sdConfig, error) {
	discovererType, _, _ := d.extractDiscovererAndName(fn.ID())
	pipelineID := pipelineKey(discovererType, name)
	key := materializationIdentity("config", []byte(discovererType+":"+name))
	return runMaterialization(
		fn.Context(),
		d,
		key,
		true,
		func(context.Context) (sdConfig, error) {
			return d.materializeDyncfgConfig(fn, discovererType, name, pipelineID)
		},
	)
}

func (d *ServiceDiscovery) testDyncfgConfig(
	fn dyncfg.Function,
	name string,
) (bool, error) {
	discovererType, _, _ := d.extractDiscovererAndName(fn.ID())
	pipelineID := pipelineKey(discovererType, name)
	key := materializationIdentity(
		"test",
		[]byte(pipelineID),
		fn.Payload(),
	)
	return runMaterialization(
		fn.Context(),
		d,
		key,
		false,
		func(ctx context.Context) (bool, error) {
			pipelineConfig, err := parseDyncfgPayload(
				fn.Payload(),
				discovererType,
				name,
				d.configDefaults,
				d.discovererRegistry(),
				true,
			)
			if err != nil {
				return false, err
			}
			pipelineConfig.Name = naming.Sanitize(name)
			pipelineConfig.Source = "dyncfg=" + fn.Source()
			prepared, err := d.constructPipeline(pipelineConfig)
			if err != nil {
				return false, err
			}
			fullyTested, err := prepared.Test(ctx)
			if err != nil {
				return false, newResourceError(resourceErrorOperationalTest, err)
			}
			return fullyTested, nil
		},
	)
}

func (d *ServiceDiscovery) materializeDyncfgConfig(
	fn dyncfg.Function,
	discovererType string,
	name string,
	pipelineID string,
) (sdConfig, error) {
	if _, err := parseDyncfgPayload(
		fn.Payload(),
		discovererType,
		name,
		d.configDefaults,
		d.discovererRegistry(),
		true,
	); err != nil {
		return nil, err
	}
	return newSDConfigFromJSON(
		fn.Payload(),
		name,
		fn.Source(),
		"dyncfg",
		discovererType,
		pipelineID,
	)
}

func (d *ServiceDiscovery) constructPipeline(config pipeline.Config) (sdPipeline, error) {
	prepared, err := d.newPipeline(config)
	if err != nil {
		return nil, newResourceError(resourceErrorConstruction, err)
	}
	return prepared, nil
}

func (d *ServiceDiscovery) preparePipeline(
	ctx context.Context,
	config sdConfig,
) (sdPipeline, error) {
	if config == nil || config.PipelineKey() == "" {
		return nil, errors.New("service discovery: invalid pipeline materialization")
	}
	identity := pipelineMaterializationIdentity(config)
	return runMaterialization(
		ctx,
		d,
		identity.Key,
		true,
		func(context.Context) (sdPipeline, error) {
			pipelineConfig, err := config.ToPipelineConfig(d.configDefaults)
			if err != nil {
				return nil, err
			}
			return d.constructPipeline(pipelineConfig)
		},
	)
}

func pipelineMaterializationIdentity(config sdConfig) jobmgr.ProcessAttemptIdentity {
	key := ""
	if config != nil {
		key = materializationIdentity("pipeline", []byte(config.ExposedKey()))
	}
	return jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptServiceDiscovery,
		Key:       key,
		Resource:  "service discovery configuration",
	}
}

func (d *ServiceDiscovery) prepareUserConfig(
	fn dyncfg.Function,
	discovererType string,
	name string,
) (pipeline.Config, error) {
	key := materializationIdentity(
		"userconfig",
		[]byte(discovererType),
		[]byte(name),
		fn.Payload(),
	)
	return runMaterialization(
		fn.Context(),
		d,
		key,
		false,
		func(context.Context) (pipeline.Config, error) {
			return parseDyncfgPayload(
				fn.Payload(),
				discovererType,
				name,
				d.configDefaults,
				d.discovererRegistry(),
				false,
			)
		},
	)
}
