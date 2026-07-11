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

// CredentialConfig describes only how the base AWS credentials are acquired.
// Which identity is monitored, including optional role assumption, is a separate
// Identity compiled from a CloudWatch target.
type CredentialConfig struct {
	Type            string `yaml:"type" json:"type"`
	AccessKeyID     string `yaml:"access_key_id,omitempty" json:"access_key_id,omitempty"`
	SecretAccessKey string `yaml:"secret_access_key,omitempty" json:"secret_access_key,omitempty"`
	SessionToken    string `yaml:"session_token,omitempty" json:"session_token,omitempty"`
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

func (c CredentialConfig) NormalizedType() string {
	return strings.ToLower(strings.TrimSpace(c.Type))
}

func (c CredentialConfig) ValidateWithPath(path string) error {
	typeField := fieldPath(path, "type")
	typ := c.NormalizedType()
	if typ == "" {
		return errors.New(typeField + " is required")
	}

	switch typ {
	case CredentialTypeDefault:
		if strings.TrimSpace(c.AccessKeyID) != "" || strings.TrimSpace(c.SecretAccessKey) != "" || strings.TrimSpace(c.SessionToken) != "" {
			return fmt.Errorf("%s %q cannot contain static credential fields", typeField, CredentialTypeDefault)
		}
	case CredentialTypeStatic:
		var errs []error
		if strings.TrimSpace(c.AccessKeyID) == "" {
			errs = append(errs, errors.New(fieldPath(path, "access_key_id")+" is required"))
		}
		if strings.TrimSpace(c.SecretAccessKey) == "" {
			errs = append(errs, errors.New(fieldPath(path, "secret_access_key")+" is required"))
		}
		return errors.Join(errs...)
	default:
		return fmt.Errorf("%s %q is invalid: expected one of %q, %q",
			typeField, c.Type, CredentialTypeDefault, CredentialTypeStatic)
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
	id := Identity{Ref: strings.TrimSpace(ref), credentials: credentials}
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
	if strings.TrimSpace(id.Ref) == "" {
		return aws.Config{}, errors.New("target reference is required")
	}
	if err := id.credentials.ValidateWithPath("credentials"); err != nil {
		return aws.Config{}, err
	}
	if id.role != nil && strings.TrimSpace(id.role.RoleARN) == "" {
		return aws.Config{}, errors.New("assume_role.role_arn is required")
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
	if id.credentials.NormalizedType() == CredentialTypeStatic {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				strings.TrimSpace(id.credentials.AccessKeyID),
				strings.TrimSpace(id.credentials.SecretAccessKey),
				strings.TrimSpace(id.credentials.SessionToken),
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
