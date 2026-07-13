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
	CredentialTypeDefault = "default"
	CredentialTypeStatic  = "static"
)

// StaticCredentialConfig contains explicit long-lived or temporary AWS credentials.
type StaticCredentialConfig struct {
	AccessKeyID     string `yaml:"access_key_id,omitempty" json:"access_key_id,omitempty"`
	SecretAccessKey string `yaml:"secret_access_key,omitempty" json:"secret_access_key,omitempty"`
	SessionToken    string `yaml:"session_token,omitempty" json:"session_token,omitempty"`
}

// CredentialConfig describes only how the base AWS credentials are acquired.
// Which identity is monitored, including optional role assumption, is a separate
// Identity compiled from a CloudWatch target.
type CredentialConfig struct {
	Type       string                  `yaml:"type" json:"type"`
	TypeStatic *StaticCredentialConfig `yaml:"type_static,omitempty" json:"type_static,omitempty"`
}

type AssumeRoleConfig struct {
	RoleARN    string `yaml:"role_arn,omitempty" json:"role_arn,omitempty"`
	ExternalID string `yaml:"external_id,omitempty" json:"external_id,omitempty"`
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

func (c CredentialConfig) ValidateWithPath(path string) error {
	typeField := fieldPath(path, "type")
	if strings.TrimSpace(c.Type) == "" {
		return errors.New(typeField + " is required")
	}
	canonicalType := strings.ToLower(strings.TrimSpace(c.Type))
	if c.Type != canonicalType {
		return fmt.Errorf("%s must use canonical value %q", typeField, canonicalType)
	}

	switch c.Type {
	case CredentialTypeDefault:
		if c.TypeStatic != nil {
			return fmt.Errorf("%s is not allowed when %s is %q", fieldPath(path, "type_static"), typeField, CredentialTypeDefault)
		}
	case CredentialTypeStatic:
		if c.TypeStatic == nil {
			return errors.New(fieldPath(path, "type_static") + " is required")
		}
		staticPath := fieldPath(path, "type_static")
		errs := []error{
			validateCredentialValue(fieldPath(staticPath, "access_key_id"), c.TypeStatic.AccessKeyID, true),
			validateCredentialValue(fieldPath(staticPath, "secret_access_key"), c.TypeStatic.SecretAccessKey, true),
			validateCredentialValue(fieldPath(staticPath, "session_token"), c.TypeStatic.SessionToken, false),
		}
		return errors.Join(errs...)
	default:
		return fmt.Errorf("%s %q is invalid: expected one of %q, %q",
			typeField, c.Type, CredentialTypeDefault, CredentialTypeStatic)
	}
	return nil
}

func validateCredentialValue(path, value string, required bool) error {
	canonical := strings.TrimSpace(value)
	if canonical == "" {
		if required {
			return errors.New(path + " is required")
		}
		return nil
	}
	if value != canonical {
		return errors.New(path + " must not contain surrounding whitespace")
	}
	return nil
}

// Identity is one compiled monitored target. Ref is the target name and remains
// distinct even when multiple identities resolve to the same AWS account.
type Identity struct {
	Ref         string
	credentials CredentialConfig
	role        *AssumeRoleConfig
}

func NewIdentity(ref string, credentials CredentialConfig, role *AssumeRoleConfig) Identity {
	id := Identity{Ref: ref, credentials: credentials}
	if role != nil {
		v := *role
		id.role = &v
	}
	return id
}

// NewConfig builds a regional aws.Config for this identity.
//
//	base identity (default or static credentials) -> SDK default
//	  credential chain (env, shared config, instance profile, IRSA/web-identity),
//	  or static access keys.
//	assumed-role identity -> the base identity assumes the role via a regional STS
//	  endpoint, cached.
func (id Identity) NewConfig(ctx context.Context, opts ConfigOptions) (aws.Config, error) {
	if id.Ref == "" {
		return aws.Config{}, errors.New("target reference is required")
	}
	if id.Ref != strings.TrimSpace(id.Ref) {
		return aws.Config{}, errors.New("target reference must not contain surrounding whitespace")
	}
	if err := id.credentials.ValidateWithPath("credentials"); err != nil {
		return aws.Config{}, err
	}
	if id.role != nil {
		if strings.TrimSpace(id.role.RoleARN) == "" {
			return aws.Config{}, errors.New("assume_role.role_arn is required")
		}
		if id.role.RoleARN != strings.TrimSpace(id.role.RoleARN) {
			return aws.Config{}, errors.New("assume_role.role_arn must not contain surrounding whitespace")
		}
		if id.role.ExternalID != strings.TrimSpace(id.role.ExternalID) {
			return aws.Config{}, errors.New("assume_role.external_id must not contain surrounding whitespace")
		}
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
	if id.credentials.Type == CredentialTypeStatic {
		static := id.credentials.TypeStatic
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				static.AccessKeyID,
				static.SecretAccessKey,
				static.SessionToken,
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
	provider := stscreds.NewAssumeRoleProvider(stsClient, id.role.RoleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = "netdata" // stable session name for legible CloudTrail entries
		if id.role.ExternalID != "" {
			o.ExternalID = aws.String(id.role.ExternalID)
		}
	})
	base.Credentials = aws.NewCredentialsCache(provider)
	return base, nil
}
