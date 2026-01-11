# 11.6 Virtualization Alerts

Virtualization alerts monitor virtual machine and hypervisor health.

## VMware vSphere Alerts

### vmware_vm_cpu_usage

Monitors VM CPU utilization against configured limits.

**Context:** `vmware.vm.cpu`
**Thresholds:** WARN > 80%, CRIT > 95%

### vmware_vm_memory_usage

Monitors VM memory consumption against configured limits.

**Context:** `vmware.vm.mem`
**Thresholds:** WARN > 80%, CRIT > 95%

### vmware_vm_status

Monitors VM power state and availability.

**Context:** `vmware.vm.status`
**Thresholds:** CRIT != poweredOn

### vmware_datastore_latency

Monitors datastore access latency.

**Context:** `vmware.ds`
**Thresholds:** WARN > 50ms

## QEMU/KVM Alerts

### kvm_vcpu_latency

Monitors VCPU scheduling latency indicating hypervisor contention.

**Context:** `k8s.kvm`
**Thresholds:** WARN > 10ms

### kvm_io_latency

Monitors disk I/O latency for virtual disks.

**Context:** `kvm.io`
**Thresholds:** WARN > 50ms

### kvm_networking_latency

Monitors network virtualization latency.

**Context:** `kvm.net`
**Thresholds:** WARN > 5ms