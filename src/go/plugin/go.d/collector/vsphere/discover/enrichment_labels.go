// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/vmware/govmomi/vim25/types"
)

const (
	tagLabelPrefix             = "vsphere_tag_"
	customAttributeLabelPrefix = "vsphere_custom_attribute_"
)

func (d Discoverer) collectEnrichmentLabels(res *rs.Resources) {
	if res == nil || d.MaxUserMetadataLabels < 1 {
		return
	}

	if d.CustomAttributeMatcher != nil {
		if err := d.collectCustomAttributeLabels(res); err != nil {
			d.warnLimited(logKeyCustomAttributeDiscoveryError, "discovering : user metadata labels : collect vSphere custom attribute labels: %v", err)
		}
	}
	if d.TagCategoryMatcher != nil {
		if err := d.collectTagLabels(res); err != nil {
			d.warnLimited(logKeyTagDiscoveryError, "discovering : user metadata labels : collect vSphere tag labels: %v", err)
		}
	}
}

func (d Discoverer) collectCustomAttributeLabels(res *rs.Resources) error {
	fields, err := d.CustomFields()
	if err != nil {
		return fmt.Errorf("collect vSphere custom attribute definitions for user metadata labels: %w", err)
	}

	namesByKey := make(map[int32]string)
	for _, field := range fields {
		if field.Name == "" || !d.CustomAttributeMatcher.Match([]byte(field.Name)) {
			continue
		}
		namesByKey[field.Key] = field.Name
	}
	if len(namesByKey) == 0 {
		return nil
	}

	for _, vm := range res.VMs {
		addCustomAttributeLabels(&vm.Labels, vm.CustomValues, namesByKey, d.MaxUserMetadataLabels)
	}
	for _, host := range res.Hosts {
		addCustomAttributeLabels(&host.Labels, host.CustomValues, namesByKey, d.MaxUserMetadataLabels)
	}
	for _, ds := range res.Datastores {
		addCustomAttributeLabels(&ds.Labels, ds.CustomValues, namesByKey, d.MaxUserMetadataLabels)
	}
	for _, cluster := range res.Clusters {
		addCustomAttributeLabels(&cluster.Labels, cluster.CustomValues, namesByKey, d.MaxUserMetadataLabels)
	}
	for _, rp := range res.ResourcePools {
		addCustomAttributeLabels(&rp.Labels, rp.CustomValues, namesByKey, d.MaxUserMetadataLabels)
	}
	for _, sp := range res.StoragePods {
		addCustomAttributeLabels(&sp.Labels, sp.CustomValues, namesByKey, d.MaxUserMetadataLabels)
	}

	return nil
}

func addCustomAttributeLabels(labels *map[string]string, values map[int32]string, namesByKey map[int32]string, maxLabels int) {
	if len(values) == 0 || len(namesByKey) == 0 {
		return
	}

	type item struct {
		name  string
		value string
	}
	var items []item
	for key, value := range values {
		name := namesByKey[key]
		if name == "" || value == "" {
			continue
		}
		items = append(items, item{name: name, value: value})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })

	for _, item := range items {
		key := metadataLabelKey(customAttributeLabelPrefix, item.name)
		addUserMetadataLabel(labels, key, item.value, maxLabels)
	}
}

func (d Discoverer) collectTagLabels(res *rs.Resources) error {
	refs := resourceRefs(res)
	if len(refs) == 0 {
		return nil
	}

	tagsByRef, err := d.TagsByRef(refs)
	if err != nil {
		return fmt.Errorf("collect vSphere tag attachments for %d inventory refs: %w", len(refs), err)
	}
	for ref, tagsByCategory := range tagsByRef {
		addTagLabels(resourceLabelsByRef(res, ref), tagsByCategory, d.TagCategoryMatcher, d.MaxUserMetadataLabels)
	}

	return nil
}

func resourceRefs(res *rs.Resources) []types.ManagedObjectReference {
	refs := make([]types.ManagedObjectReference, 0,
		len(res.VMs)+len(res.Hosts)+len(res.Datastores)+len(res.Clusters)+len(res.ResourcePools)+len(res.StoragePods))
	for _, vm := range res.VMs {
		refs = append(refs, vm.Ref)
	}
	for _, host := range res.Hosts {
		refs = append(refs, host.Ref)
	}
	for _, ds := range res.Datastores {
		refs = append(refs, ds.Ref)
	}
	for _, cluster := range res.Clusters {
		refs = append(refs, cluster.Ref)
	}
	for _, rp := range res.ResourcePools {
		refs = append(refs, rp.Ref)
	}
	for _, sp := range res.StoragePods {
		refs = append(refs, sp.Ref)
	}
	return refs
}

func resourceLabelsByRef(res *rs.Resources, ref types.ManagedObjectReference) *map[string]string {
	if res == nil {
		return nil
	}
	switch ref.Type {
	case "VirtualMachine":
		if vm := res.VMs.Get(ref.Value); vm != nil {
			return &vm.Labels
		}
	case "HostSystem":
		if host := res.Hosts.Get(ref.Value); host != nil {
			return &host.Labels
		}
	case "Datastore":
		if ds := res.Datastores.Get(ref.Value); ds != nil {
			return &ds.Labels
		}
	case "ResourcePool":
		if rp := res.ResourcePools.Get(ref.Value); rp != nil {
			return &rp.Labels
		}
	case "StoragePod":
		if sp := res.StoragePods.Get(ref.Value); sp != nil {
			return &sp.Labels
		}
	}
	if cluster := res.Clusters.Get(ref.Value); cluster != nil {
		return &cluster.Labels
	}
	return nil
}

func addTagLabels(labels *map[string]string, tagsByCategory map[string][]string, categoryMatcher interface{ Match([]byte) bool }, maxLabels int) {
	if labels == nil || len(tagsByCategory) == 0 || categoryMatcher == nil {
		return
	}

	categories := make([]string, 0, len(tagsByCategory))
	for category := range tagsByCategory {
		if category != "" && categoryMatcher.Match([]byte(category)) {
			categories = append(categories, category)
		}
	}
	sort.Strings(categories)

	for _, category := range categories {
		tags := append([]string(nil), tagsByCategory[category]...)
		sort.Strings(tags)
		tags = compactNonEmptyStrings(tags)
		if len(tags) == 0 {
			continue
		}
		value := strings.Join(tags, "|")
		key := metadataLabelKey(tagLabelPrefix, category)
		addUserMetadataLabel(labels, key, value, maxLabels)
	}
}

func metadataLabelKey(prefix, name string) string {
	suffix := sanitizeLabelKeyPart(name)
	if suffix == "" {
		return ""
	}
	return prefix + suffix
}

func addUserMetadataLabel(labels *map[string]string, key, value string, maxLabels int) {
	if key == "" || value == "" || maxLabels < 1 {
		return
	}
	if *labels == nil {
		*labels = make(map[string]string)
	}
	if _, exists := (*labels)[key]; !exists && len(*labels) >= maxLabels {
		return
	}
	(*labels)[key] = value
}

func compactNonEmptyStrings(values []string) []string {
	out := values[:0]
	var last string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}

func sanitizeLabelKeyPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	b.Grow(len(value))
	lastUnderscore := false
	for _, r := range value {
		ok := r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}
