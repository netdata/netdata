// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"strings"

	"github.com/google/uuid"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	vsphereVnodeTypeLabel = "_vnode_type"
	vsphereESXIVnodeType  = "vsphere_esxi"
	vsphereVMVnodeType    = "vsphere_vm"
)

func (c *Collector) resourceHostScope(resourceID string) metrix.HostScope {
	if c.resources == nil {
		return metrix.HostScope{}
	}
	if c.ESXIVnodes {
		if host := c.resources.Hosts.Get(resourceID); host != nil {
			return c.esxiHostScope(host)
		}
	}
	if c.VMVnodes {
		if vm := c.resources.VMs.Get(resourceID); vm != nil {
			return c.vmHostScope(vm)
		}
	}
	return metrix.HostScope{}
}

func (c *Collector) esxiHostScope(host *rs.Host) metrix.HostScope {
	if host == nil {
		return metrix.HostScope{}
	}
	return c.newResourceHostScope("esxi", host.ID, host.Name, map[string]string{
		vsphereVnodeTypeLabel: vsphereESXIVnodeType,
		"vsphere_id":          host.ID,
		"datacenter":          host.Hier.DC.Name,
		"cluster":             host.Hier.Cluster.Name,
	})
}

func (c *Collector) vmHostScope(vm *rs.VM) metrix.HostScope {
	if vm == nil {
		return metrix.HostScope{}
	}
	return c.newResourceHostScope("vm", vm.ID, vm.Name, map[string]string{
		vsphereVnodeTypeLabel: vsphereVMVnodeType,
		"vsphere_id":          vm.ID,
		"datacenter":          vm.Hier.DC.Name,
		"cluster":             vm.Hier.Cluster.Name,
		"host":                vm.Hier.Host.Name,
	})
}

func (c *Collector) newResourceHostScope(kind, resourceID, hostname string, labels map[string]string) metrix.HostScope {
	kind = strings.TrimSpace(kind)
	resourceID = strings.TrimSpace(resourceID)
	hostname = strings.TrimSpace(hostname)
	if kind == "" || resourceID == "" || hostname == "" {
		return metrix.HostScope{}
	}

	vcenterID := strings.TrimSpace(c.vcenterInstanceUUID)
	if vcenterID == "" {
		return metrix.HostScope{}
	}

	guid := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("vsphere:"+vcenterID+":"+kind+":"+resourceID)).String()
	scope := metrix.HostScope{
		ScopeKey: guid,
		GUID:     guid,
		Hostname: hostname,
		Labels:   labels,
	}
	if _, err := chartemit.PrepareHostInfo(netdataapi.HostInfo{
		GUID:     scope.GUID,
		Hostname: scope.Hostname,
		Labels:   scope.Labels,
	}); err != nil {
		return metrix.HostScope{}
	}
	return scope
}
