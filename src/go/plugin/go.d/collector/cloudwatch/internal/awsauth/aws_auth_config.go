// SPDX-License-Identifier: GPL-3.0-or-later

package awsauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	ModeDefault    = "default"
	ModeAccessKey  = "access_key"
	ModeAssumeRole = "assume_role"

	// defaultConfigPath is the error-message prefix Validate() uses when no explicit
	// path is given. cloudwatch (the sole consumer) embeds this config under the
	// "auth" key, so validation errors read as "auth.*".
	defaultConfigPath = "auth"

	// baseIdentityRef is Identity.Ref for the base credential source when it is
	// monitored alongside assumed roles (include_base_account).
	baseIdentityRef = "base"
)

type ModeAccessKeyConfig struct {
	AccessKeyID     string `yaml:"access_key_id,omitempty" json:"access_key_id,omitempty"`
	SecretAccessKey string `yaml:"secret_access_key,omitempty" json:"secret_access_key,omitempty"`
	SessionToken    string `yaml:"session_token,omitempty" json:"session_token,omitempty"`
}

type AssumeRole struct {
	RoleARN    string `yaml:"role_arn,omitempty" json:"role_arn,omitempty"`
	ExternalID string `yaml:"external_id,omitempty" json:"external_id,omitempty"`
}

type ModeAssumeRoleConfig struct {
	// Roles are assumed one per monitored account; account_id is resolved per role
	// via sts:GetCallerIdentity.
	Roles []AssumeRole `yaml:"roles,omitempty" json:"roles,omitempty"`
	// IncludeBaseAccount also monitors the base identity's own account (the identity
	// used to assume the roles). Off by default: with roles set, only the assumed-role
	// accounts are monitored.
	IncludeBaseAccount bool `yaml:"include_base_account,omitempty" json:"include_base_account,omitempty"`
}

type Config struct {
	Mode           string                `yaml:"mode,omitempty" json:"mode,omitempty"`
	ModeAccessKey  *ModeAccessKeyConfig  `yaml:"mode_access_key,omitempty" json:"mode_access_key,omitempty"`
	ModeAssumeRole *ModeAssumeRoleConfig `yaml:"mode_assume_role,omitempty" json:"mode_assume_role,omitempty"`
}

// ConfigOptions controls how the regional aws.Config is built.
type ConfigOptions struct {
	// Region is the AWS region the resulting config targets.
	Region string
	// STSRegion overrides the STS endpoint region for assume_role mode.
	// Defaults to Region (a regional endpoint, which also works in gov/cn
	// partitions where the global STS endpoint does not exist).
	STSRegion string
}

func (c Config) NormalizedMode() string {
	return strings.ToLower(strings.TrimSpace(c.Mode))
}

func (c Config) Validate() error {
	return c.ValidateWithPath(defaultConfigPath)
}

func (c Config) ValidateWithPath(path string) error {
	modeField := fieldPath(path, "mode")
	mode := c.NormalizedMode()

	if mode == "" {
		return errors.New(modeField + " is required")
	}

	switch mode {
	case ModeDefault:
		return nil
	case ModeAccessKey:
		if c.ModeAccessKey == nil {
			return fmt.Errorf("%s is required when %s is %q", fieldPath(path, "mode_access_key"), modeField, ModeAccessKey)
		}
		var errs []error
		if strings.TrimSpace(c.ModeAccessKey.AccessKeyID) == "" {
			errs = append(errs, errors.New(fieldPath(path, "mode_access_key.access_key_id")+" is required"))
		}
		if strings.TrimSpace(c.ModeAccessKey.SecretAccessKey) == "" {
			errs = append(errs, errors.New(fieldPath(path, "mode_access_key.secret_access_key")+" is required"))
		}
		return errors.Join(errs...)
	case ModeAssumeRole:
		rolesField := fieldPath(path, "mode_assume_role.roles")
		if c.ModeAssumeRole == nil || len(c.ModeAssumeRole.Roles) == 0 {
			return fmt.Errorf("%s must contain at least one role when %s is %q", rolesField, modeField, ModeAssumeRole)
		}
		// One role per monitored account; each needs a role_arn.
		var errs []error
		for i, r := range c.ModeAssumeRole.Roles {
			if strings.TrimSpace(r.RoleARN) == "" {
				errs = append(errs, errors.New(fieldPath(path, fmt.Sprintf("mode_assume_role.roles[%d].role_arn", i))+" is required"))
			}
		}
		return errors.Join(errs...)
	default:
		return fmt.Errorf("%s %q is invalid: expected one of %q, %q, %q",
			modeField, c.Mode, ModeDefault, ModeAccessKey, ModeAssumeRole)
	}
}

// Identity is one AWS credential source the collector treats as a distinct account.
// Build a regional aws.Config for it with NewConfig.
type Identity struct {
	// Ref is a stable, config-derived reference used for logging and as a pre-STS
	// cache key: the role ARN for an assumed-role identity, or the mode name
	// (default/access_key) or "base" for a base identity.
	Ref  string
	auth Config
	role *AssumeRole // non-nil => assume this role; nil => base identity (no assumption)
}

// Identities returns the distinct credential sources this config monitors: exactly
// one for default/access_key; one per role for assume_role, plus the base identity
// when include_base_account is set. The order is stable (roles as configured, base
// last).
func (c Config) Identities() []Identity {
	if c.NormalizedMode() == ModeAssumeRole && c.ModeAssumeRole != nil {
		ids := make([]Identity, 0, len(c.ModeAssumeRole.Roles)+1)
		for i := range c.ModeAssumeRole.Roles {
			ids = append(ids, Identity{
				Ref:  strings.TrimSpace(c.ModeAssumeRole.Roles[i].RoleARN),
				auth: c,
				role: &c.ModeAssumeRole.Roles[i],
			})
		}
		if c.ModeAssumeRole.IncludeBaseAccount {
			ids = append(ids, Identity{Ref: baseIdentityRef, auth: c})
		}
		return ids
	}
	return []Identity{{Ref: c.NormalizedMode(), auth: c}}
}

// NewConfig builds a regional aws.Config for this identity.
//
//	base identity (default / access_key / the included base) -> SDK default
//	  credential chain (env, shared config, instance profile, IRSA/web-identity),
//	  or static access keys.
//	assumed-role identity -> the base identity assumes the role via a regional STS
//	  endpoint, cached.
func (id Identity) NewConfig(ctx context.Context, opts ConfigOptions) (aws.Config, error) {
	if err := id.auth.Validate(); err != nil {
		return aws.Config{}, err
	}

	region := strings.TrimSpace(opts.Region)

	loadOpts := []func(*awsconfig.LoadOptions) error{
		// Bounded retries with a capped backoff: the SDK default (3 attempts, 20s
		// max backoff) could let a throttled call exceed the collector timeout.
		awsconfig.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = 5
				o.MaxBackoff = 3 * time.Second
			})
		}),
	}
	if region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(region))
	}
	if id.auth.NormalizedMode() == ModeAccessKey {
		ak := id.auth.ModeAccessKey
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				strings.TrimSpace(ak.AccessKeyID),
				strings.TrimSpace(ak.SecretAccessKey),
				strings.TrimSpace(ak.SessionToken),
			),
		))
	}

	base, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return aws.Config{}, err
	}
	if id.role == nil {
		return base, nil // base identity — no role assumption
	}

	stsRegion := strings.TrimSpace(opts.STSRegion)
	if stsRegion == "" {
		stsRegion = region
	}
	stsClient := sts.NewFromConfig(base, func(o *sts.Options) {
		if stsRegion != "" {
			o.Region = stsRegion
		}
	})
	provider := stscreds.NewAssumeRoleProvider(stsClient, strings.TrimSpace(id.role.RoleARN), func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = "netdata" // stable session name for legible CloudTrail entries
		if v := strings.TrimSpace(id.role.ExternalID); v != "" {
			o.ExternalID = aws.String(v)
		}
	})
	base.Credentials = aws.NewCredentialsCache(provider)
	return base, nil
}
