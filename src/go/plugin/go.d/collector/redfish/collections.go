// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/stmcginnis/gofish/schemas"
)

// A malformed member must not hide valid siblings. Only references are used:
// each resource is fetched directly, without expansion or partial inline data.
type collectionReference struct {
	schemas.Entity
}

func (r *collectionReference) UnmarshalJSON(raw []byte) error {
	var link struct {
		ODataID string `json:"@odata.id"`
	}
	if json.Unmarshal(raw, &link) == nil {
		r.ODataID = link.ODataID
	}
	return nil
}

func (c *protocolClient) fetchCollectionMembers(
	ctx context.Context,
	ref string,
	stats *wireStats,
) ([]collectionMember, bool, error) {
	target, err := c.resolveURI(c.root, ref, false)
	if err != nil {
		return nil, false, err
	}
	return c.fetchCollectionMemberPages(ctx, target, nil, stats)
}

// A failed listing retains prior identities as unknown; a complete listing is
// authoritative even if fetching an individual member subsequently fails.
func (c *protocolClient) fetchCollectionMemberPages(
	ctx context.Context,
	target *url.URL,
	first *responseData,
	stats *wireStats,
) ([]collectionMember, bool, error) {
	var members []collectionMember
	pages, seen := make(map[string]bool), make(map[string]bool)
	var invalid boundedErrorAccumulator
	for target != nil {
		if err := contextError(ctx); err != nil {
			return members, false, err
		}
		if pages[target.String()] {
			return members, false, errors.New("collection pagination loop")
		}
		pages[target.String()] = true
		response := first
		first = nil
		if response == nil {
			var err error
			response, err = c.get(ctx, target, stats)
			if err != nil {
				return members, false, err
			}
		}
		var page schemas.ResourceCollectionGeneric[*collectionReference]
		err := decodeJSON(response, &page)
		if err == nil && page.Members == nil {
			err = errors.New("collection page has no Members array")
		}
		response.finish(err)
		if err != nil {
			return members, false, err
		}
		for _, member := range page.Members {
			if member == nil || member.ODataID == "" {
				invalid.Add(errors.New("collection member has no usable @odata.id"))
				continue
			}
			link, err := c.resolveURI(response.url, member.ODataID, false)
			if err != nil {
				invalid.Add(err)
				continue
			}
			key := canonicalResourceURI(link)
			if seen[key] {
				continue
			}
			seen[key] = true
			members = append(members, collectionMember{
				Ref: redfishLink{
					ODataID: key,
				},
			})
		}
		if page.MembersNextLink == "" {
			break
		}
		target, err = c.resolveURI(response.url, page.MembersNextLink, true)
		if err != nil {
			return members, false, fmt.Errorf("collection next page: %w", err)
		}
	}
	err := invalid.Err()
	return members, err == nil, err
}
