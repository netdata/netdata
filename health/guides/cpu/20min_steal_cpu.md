### Understand the alert

This alarm calculates average CPU `steal` time over the last 20 minutes

`steal`, in a virtual machine, is the percentage of time that particular virtual CPU has to wait for an available host CPU to run on. If this metric goes up, it means that your VM is not getting the processing power it needs.

### Troubleshoot the alert

Check for CPU quota and host issues.

Generally, if `steal` is high, it could mean one of the following:

- Another VM on the host system is hogging the CPU.
- System services on the host system are monopolizing the CPU (for example, system updates).
- The host CPUs are over-committed (you have more virtual CPUs assigned to VMs than the host system has physical CPUs) and too many VMs need CPU time simultanously.
- The VM itself has a CPU quota that is too low.

So in the end you can increase the CPU resources of that particular VM, and if the alert persists, move the guest to a different *physical* server.
