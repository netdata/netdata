// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

func (c *protocolClient) fetchCollectionMembers(
	ctx context.Context,
	ref string,
	kind string,
	stats *wireStats,
) ([]collectionMember, bool, error) {
	target, err := c.resolveURI(c.root, ref, false)
	if err != nil {
		return nil, false, err
	}
	return c.fetchCollectionMembersAt(ctx, target, nil, kind, stats)
}

func (c *protocolClient) fetchCollectionMembersAt(
	ctx context.Context,
	target *url.URL,
	first *responseData,
	kind string,
	stats *wireStats,
) ([]collectionMember, bool, error) {
	identity := canonicalResourceURI(target)
	c.markKnownCollection(identity)
	if first == nil && kind != "" {
		if expand := c.collectionExpansion(identity); expand != "" {
			expanded := *target
			query := make(url.Values, 1)
			query.Set("$expand", expand)
			expanded.RawQuery = query.Encode()
			members, complete, err := c.fetchCollectionMemberPages(
				ctx,
				&expanded,
				nil,
				stats,
				kind,
				true,
			)
			if err == nil && complete {
				return members, true, nil
			}
			if persistentCollectionExpansionFailure(err) {
				c.disableCollectionExpansion(identity, expand)
			}
			if isCallerContextError(err) {
				return members, false, err
			}
		}
	}
	return c.fetchCollectionMemberPages(
		ctx,
		target,
		first,
		stats,
		kind,
		false,
	)
}

func persistentCollectionExpansionFailure(err error) bool {
	if err == nil || isCallerContextError(err) {
		return false
	}
	var status statusError
	if errors.As(err, &status) {
		switch status.status {
		case http.StatusBadRequest,
			http.StatusNotFound,
			http.StatusMethodNotAllowed,
			http.StatusNotAcceptable,
			http.StatusRequestURITooLong,
			http.StatusUnsupportedMediaType,
			http.StatusUnprocessableEntity,
			http.StatusNotImplemented:
			return true
		default:
			return false
		}
	}
	switch classifyError(err) {
	case "auth", "tls", "transport", "timeout", "limit":
		return false
	default:
		return true
	}
}

func (c *protocolClient) fetchCollectionMemberPages(
	ctx context.Context,
	target *url.URL,
	first *responseData,
	stats *wireStats,
	expectedKind string,
	requireExpanded bool,
) ([]collectionMember, bool, error) {
	progress := collectionProgress{
		CollectionIdentity: canonicalResourceURI(target),
		ExpectedCount:      -1,
		SeenPages:          make(map[string]struct{}),
		SeenMembers:        make(map[string]struct{}),
	}
	for target != nil {
		if err := contextError(ctx); err != nil {
			return progress.Members, false, err
		}
		pageKey := target.String()
		if _, seen := progress.SeenPages[pageKey]; seen {
			return progress.Members, false, errors.New("collection pagination loop")
		}
		response := first
		first = nil
		if response == nil {
			var err error
			response, err = c.do(
				ctx,
				protocolRequest{
					method: http.MethodGet,
					target: target,
					auth:   c.currentAuth(true),
				},
				stats,
				true,
				http.StatusOK,
			)
			if err != nil {
				return progress.Members, false, err
			}
		}
		next, err := c.collectCollectionPage(response, pageKey, expectedKind, requireExpanded, &progress)
		if err != nil {
			return progress.Members, false, err
		}
		target = next
	}
	return progress.Members, true, nil
}

// Each acquired page is accounted once, after its body and membership are validated.
func (c *protocolClient) collectCollectionPage(
	response *responseData,
	pageKey, kind string,
	expanded bool,
	progress *collectionProgress,
) (next *url.URL, err error) {
	defer func() { response.finish(err) }()
	page, rawMembers, err := c.decodeCollectionPage(response, kind, progress)
	if err != nil {
		return nil, err
	}
	for _, raw := range rawMembers {
		member, memberErr := c.decodeCollectionMember(response, raw, kind, expanded, progress.SeenMembers)
		if memberErr != nil {
			if expanded {
				return nil, memberErr
			}
			progress.recordInvalidMember(memberErr)
			continue
		}
		progress.SeenMembers[member.Ref.ODataID] = struct{}{}
		progress.Members = append(progress.Members, member)
	}
	progress.SeenPages[pageKey] = struct{}{}
	if page.NextLink == "" {
		return nil, progress.completionError()
	}
	return c.resolveURI(response.url, page.NextLink, true)
}

func (c *protocolClient) decodeCollectionPage(
	response *responseData,
	kind string,
	progress *collectionProgress,
) (collectionPage, []json.RawMessage, error) {
	var page collectionPage
	if err := decodeJSON(response, &page); err != nil {
		return page, nil, err
	}
	if len(progress.SeenPages) == 0 {
		progress.CollectionIdentity = canonicalResourceURI(response.url)
	}
	resolved, err := c.resolveURI(response.url, page.ODataID, false)
	if err != nil || !sameResourceIdentity(canonicalResourceURI(resolved), progress.CollectionIdentity) {
		return page, nil, errors.New("collection identity does not match the requested collection")
	}
	if err := validateCollectionSchemaType(page.ODataType, kind); err != nil {
		return page, nil, err
	}
	if page.Count == nil || *page.Count < 0 {
		return page, nil, errors.New("collection page has no valid @odata.count")
	}
	if progress.ExpectedCount < 0 {
		progress.ExpectedCount = *page.Count
	} else if progress.ExpectedCount != *page.Count {
		return page, nil, errors.New("collection @odata.count changed between pages")
	}
	if len(page.Members) == 0 || bytes.Equal(bytes.TrimSpace(page.Members), []byte("null")) {
		return page, nil, errors.New("collection page has no Members array")
	}
	var members []json.RawMessage
	if err := json.Unmarshal(page.Members, &members); err != nil {
		return page, nil, errors.New("collection Members is not an array")
	}
	return page, members, nil
}

func (c *protocolClient) decodeCollectionMember(
	response *responseData,
	raw json.RawMessage,
	kind string,
	expanded bool,
	seen map[string]struct{},
) (collectionMember, error) {
	var data map[string]any
	if err := decodeJSONBytes(raw, &data); err != nil || data == nil {
		return collectionMember{}, errors.New("collection member is not an object")
	}
	rawURI, ok := stringValue(data["@odata.id"])
	if !ok {
		return collectionMember{}, errors.New("collection member has no @odata.id")
	}
	target, err := c.resolveURI(response.url, rawURI, false)
	if err != nil {
		return collectionMember{}, err
	}
	key := canonicalResourceURI(target)
	if _, duplicate := seen[key]; duplicate {
		return collectionMember{}, errors.New("collection contains duplicate member identity")
	}
	member := collectionMember{
		Ref: redfishLink{
			ODataID: key,
		},
	}
	if expanded {
		if kind == "" {
			return collectionMember{}, errors.New("expanded collection member has no expected resource kind")
		}
		if err := c.validateResourceIdentity(kind, data, target); err != nil {
			return collectionMember{}, err
		}
		member.Data, member.Response = data, metadataForResponse(response)
	}
	return member, nil
}

func (p *collectionProgress) completionError() error {
	var err error
	if p.InvalidMembers > 0 {
		err = fmt.Errorf("collection skipped %d invalid or duplicate members: %s", p.InvalidMembers, p.FirstMemberError)
	}
	if len(p.Members) != p.ExpectedCount {
		err = errors.Join(
			err,
			fmt.Errorf("collection has %d unique members, advertised %d", len(p.Members), p.ExpectedCount),
		)
	}
	return err
}

func (p *collectionProgress) recordInvalidMember(err error) {
	p.InvalidMembers++
	if p.FirstMemberError == "" {
		p.FirstMemberError = err.Error()
	}
}

func isCallerContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func expansionValue(root *serviceRootDocument) string {
	if root == nil {
		return ""
	}
	expand := root.ProtocolFeaturesSupported.ExpandQuery
	switch {
	case expand.NoLinks:
		return "."
	case expand.Links:
		return "~"
	case expand.ExpandAll:
		return "*"
	default:
		return ""
	}
}

func (c *protocolClient) setExpansionValue(value string) {
	c.collectionMu.Lock()
	c.ensureCollectionStateLocked()
	c.expansionValue = value
	c.collectionMu.Unlock()
}

func (c *protocolClient) collectionExpansion(identity string) string {
	c.collectionMu.Lock()
	defer c.collectionMu.Unlock()
	c.ensureCollectionStateLocked()
	if c.expansionValue == "" {
		return ""
	}
	key := identity + "\x00" + c.expansionValue
	if _, disabled := c.expansionDisabled[key]; disabled {
		return ""
	}
	return c.expansionValue
}

func (c *protocolClient) disableCollectionExpansion(identity, value string) {
	c.collectionMu.Lock()
	c.ensureCollectionStateLocked()
	c.expansionDisabled[identity+"\x00"+value] = struct{}{}
	c.expansionFallbackSeen = true
	c.collectionMu.Unlock()
}

func (c *protocolClient) takeExpansionFallbackDiagnostic() string {
	c.collectionMu.Lock()
	defer c.collectionMu.Unlock()
	if !c.expansionFallbackSeen {
		return ""
	}
	c.expansionFallbackSeen = false
	return "Redfish compatibility: advertised collection query expansion was rejected; using ordinary member links"
}

func (c *protocolClient) markKnownCollection(identity string) {
	c.collectionMu.Lock()
	c.ensureCollectionStateLocked()
	c.knownCollections[identity] = struct{}{}
	c.collectionMu.Unlock()
}

func (c *protocolClient) isKnownCollection(identity string) bool {
	c.collectionMu.Lock()
	defer c.collectionMu.Unlock()
	c.ensureCollectionStateLocked()
	_, ok := c.knownCollections[identity]
	return ok
}

func (c *protocolClient) ensureCollectionStateLocked() {
	if c.knownCollections == nil {
		c.knownCollections = make(map[string]struct{})
	}
	if c.expansionDisabled == nil {
		c.expansionDisabled = make(map[string]struct{})
	}
}
