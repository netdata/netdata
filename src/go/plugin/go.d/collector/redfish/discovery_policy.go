// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// ResolveDiscoveryRedirect applies the endpoint collector's ordinary-resource
// URI boundary to one unauthenticated discovery redirect.
func ResolveDiscoveryRedirect(
	configuredRoot string,
	current *url.URL,
	location string,
) (*url.URL, error) {
	root, origin, err := normalizeServiceRoot(configuredRoot)
	if err != nil {
		return nil, err
	}
	if _, err := resolveRedfishURI(origin, root, current.String(), uriResource); err != nil {
		return nil, err
	}
	return resolveRedfishURI(origin, current, location, uriResource)
}

// ValidateDiscoveryServiceRoot validates enough of a credential-free
// ServiceRoot to ensure that a rendered authenticated job still targets the
// exact Redfish origin and standardized root path that discovery probed.
func ValidateDiscoveryServiceRoot(
	payload []byte,
	responseURL *url.URL,
	configuredRoot string,
) error {
	rootURL, origin, err := normalizeServiceRoot(configuredRoot)
	if err != nil {
		return err
	}
	finalURL, err := resolveRedfishURI(origin, rootURL, responseURL.String(), uriResource)
	if err != nil {
		return fmt.Errorf("invalid ServiceRoot response URI: %w", err)
	}
	if strings.TrimSuffix(canonicalResourceURI(finalURL), "/") != "/redfish/v1" {
		return errors.New("ServiceRoot response left the standardized root path")
	}

	var root struct {
		ODataID        string `json:"@odata.id"`
		ODataType      string `json:"@odata.type"`
		ID             string `json:"Id"`
		Name           string `json:"Name"`
		RedfishVersion string `json:"RedfishVersion"`
		Systems        redfishLink
		Chassis        redfishLink
		Managers       redfishLink
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return fmt.Errorf("decode ServiceRoot: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("decode ServiceRoot: response contains trailing JSON data")
	}
	identity, err := resolveRedfishURI(origin, finalURL, root.ODataID, uriResource)
	if err != nil ||
		!sameResourceIdentity(canonicalResourceURI(identity), canonicalResourceURI(finalURL)) {
		return errors.New("ServiceRoot @odata.id does not identify the response URI")
	}
	if err := validateResourceSchemaType("service", root.ODataType); err != nil {
		return err
	}
	if strings.TrimSpace(root.ID) == "" || strings.TrimSpace(root.Name) == "" {
		return errors.New("ServiceRoot has no usable Id or Name")
	}
	if !validRedfishVersion(root.RedfishVersion) {
		return errors.New("ServiceRoot has no valid RedfishVersion")
	}
	for _, link := range []redfishLink{root.Systems, root.Chassis, root.Managers} {
		if link.ODataID == "" {
			continue
		}
		if _, err := resolveRedfishURI(origin, finalURL, link.ODataID, uriResource); err == nil {
			return nil
		}
	}
	return errors.New("ServiceRoot has no valid Systems, Chassis, or Managers link")
}
