// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

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
	AWSAuthModeDefault    = "default"
	AWSAuthModeAccessKey  = "access_key"
	AWSAuthModeAssumeRole = "assume_role"

	awsAuthConfigPath = "cloud_auth.aws"
)

type AWSModeAccessKeyConfig struct {
	AccessKeyID     string `yaml:"access_key_id,omitempty" json:"access_key_id,omitempty"`
	SecretAccessKey string `yaml:"secret_access_key,omitempty" json:"secret_access_key,omitempty"`
	SessionToken    string `yaml:"session_token,omitempty" json:"session_token,omitempty"`
}

type AWSAssumeRole struct {
	RoleARN    string `yaml:"role_arn,omitempty" json:"role_arn,omitempty"`
	ExternalID string `yaml:"external_id,omitempty" json:"external_id,omitempty"`
}

type AWSModeAssumeRoleConfig struct {
	Roles []AWSAssumeRole `yaml:"roles,omitempty" json:"roles,omitempty"`
}

type AWSAuthConfig struct {
	Mode           string                   `yaml:"mode,omitempty" json:"mode,omitempty"`
	ModeAccessKey  *AWSModeAccessKeyConfig  `yaml:"mode_access_key,omitempty" json:"mode_access_key,omitempty"`
	ModeAssumeRole *AWSModeAssumeRoleConfig `yaml:"mode_assume_role,omitempty" json:"mode_assume_role,omitempty"`
}

// AWSConfigOptions controls how the regional aws.Config is built.
type AWSConfigOptions struct {
	// Region is the AWS region the resulting config targets.
	Region string
	// STSRegion overrides the STS endpoint region for assume_role mode.
	// Defaults to Region (a regional endpoint, which also works in gov/cn
	// partitions where the global STS endpoint does not exist).
	STSRegion string
}

func (c AWSAuthConfig) NormalizedMode() string {
	return strings.ToLower(strings.TrimSpace(c.Mode))
}

func (c AWSAuthConfig) Validate() error {
	return c.ValidateWithPath(awsAuthConfigPath)
}

func (c AWSAuthConfig) ValidateWithPath(path string) error {
	modeField := fieldPath(path, "mode")
	mode := c.NormalizedMode()

	if mode == "" {
		return errors.New(modeField + " is required")
	}

	switch mode {
	case AWSAuthModeDefault:
		return nil
	case AWSAuthModeAccessKey:
		if c.ModeAccessKey == nil {
			return fmt.Errorf("%s is required when %s is %q", fieldPath(path, "mode_access_key"), modeField, AWSAuthModeAccessKey)
		}
		var errs []error
		if strings.TrimSpace(c.ModeAccessKey.AccessKeyID) == "" {
			errs = append(errs, errors.New(fieldPath(path, "mode_access_key.access_key_id")+" is required"))
		}
		if strings.TrimSpace(c.ModeAccessKey.SecretAccessKey) == "" {
			errs = append(errs, errors.New(fieldPath(path, "mode_access_key.secret_access_key")+" is required"))
		}
		return errors.Join(errs...)
	case AWSAuthModeAssumeRole:
		rolesField := fieldPath(path, "mode_assume_role.roles")
		if c.ModeAssumeRole == nil || len(c.ModeAssumeRole.Roles) == 0 {
			return fmt.Errorf("%s must contain at least one role when %s is %q", rolesField, modeField, AWSAuthModeAssumeRole)
		}
		// MVP supports a single role; the schema is a list from day one so
		// multi-account fan-out can be enabled later without a breaking change.
		if len(c.ModeAssumeRole.Roles) != 1 {
			return fmt.Errorf("%s currently supports exactly one role", rolesField)
		}
		if strings.TrimSpace(c.ModeAssumeRole.Roles[0].RoleARN) == "" {
			return errors.New(fieldPath(path, "mode_assume_role.roles[0].role_arn") + " is required")
		}
		return nil
	default:
		return fmt.Errorf("%s %q is invalid: expected one of %q, %q, %q",
			modeField, c.Mode, AWSAuthModeDefault, AWSAuthModeAccessKey, AWSAuthModeAssumeRole)
	}
}

// NewConfig builds a regional aws.Config for the configured auth mode.
//
// default     -> SDK default credential chain (env, shared config, instance
//
//	profile, IRSA/web-identity).
//
// access_key  -> static credentials.
// assume_role -> base identity assumes the configured role via a regional STS
//
//	endpoint, cached.
func (c AWSAuthConfig) NewConfig(ctx context.Context, opts AWSConfigOptions) (aws.Config, error) {
	if err := c.Validate(); err != nil {
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

	switch c.NormalizedMode() {
	case AWSAuthModeAccessKey:
		ak := c.ModeAccessKey
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				strings.TrimSpace(ak.AccessKeyID),
				strings.TrimSpace(ak.SecretAccessKey),
				strings.TrimSpace(ak.SessionToken),
			),
		))
		return awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	case AWSAuthModeAssumeRole:
		base, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
		if err != nil {
			return aws.Config{}, err
		}
		role := c.ModeAssumeRole.Roles[0]
		stsRegion := strings.TrimSpace(opts.STSRegion)
		if stsRegion == "" {
			stsRegion = region
		}
		stsClient := sts.NewFromConfig(base, func(o *sts.Options) {
			if stsRegion != "" {
				o.Region = stsRegion
			}
		})
		provider := stscreds.NewAssumeRoleProvider(stsClient, strings.TrimSpace(role.RoleARN), func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = "netdata" // stable session name for legible CloudTrail entries
			if v := strings.TrimSpace(role.ExternalID); v != "" {
				o.ExternalID = aws.String(v)
			}
		})
		base.Credentials = aws.NewCredentialsCache(provider)
		return base, nil
	default: // AWSAuthModeDefault
		return awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	}
}
