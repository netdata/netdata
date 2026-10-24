// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"errors"
	"slices"
	"strings"

	"github.com/stmcginnis/gofish/schemas"
)

func (c *logClient) services() (map[string]*schemas.LogService, error) {
	services := make(map[string]*schemas.LogService)
	for _, discover := range []func() error{
		func() error { return discoverOwnerLogs(c.Service.Systems, services) },
		func() error { return discoverOwnerLogs(c.Service.Managers, services) },
		func() error { return discoverOwnerLogs(c.Service.Chassis, services) },
	} {
		if err := discover(); err != nil {
			return nil, errors.New(
				"could not discover all Redfish log services; check BMC availability and read permissions",
			)
		}
	}
	normalized := make(map[string]*schemas.LogService, len(services))
	for uri, service := range services {
		target, err := resolveRedfishURI(c.connection.origin, c.connection.root, uri, uriResource)
		if err != nil {
			return nil, errors.New("Redfish log service has an invalid resource URI")
		}
		normalized[canonicalResourceURI(target)] = service
	}
	return normalized, nil
}

func discoverOwnerLogs[T interface {
	LogServices() ([]*schemas.LogService, error)
}](get func() ([]T, error), services map[string]*schemas.LogService) error {
	owners, err := get()
	if err != nil {
		return err
	}
	for _, owner := range owners {
		logs, err := owner.LogServices()
		if err != nil {
			return err
		}
		for _, service := range logs {
			services[service.ODataID] = service
		}
	}
	return nil
}

func logServiceChoices(services map[string]*schemas.LogService) []LogService {
	result := make([]LogService, 0, len(services))
	for uri, s := range services {
		result = append(result, LogService{
			URI:  uri,
			Name: s.Name,
		})
	}
	slices.SortFunc(result, func(a, b LogService) int { return strings.Compare(a.URI, b.URI) })
	return result
}
