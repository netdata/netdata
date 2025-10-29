// SPDX-License-Identifier: GPL-3.0-or-later

#include <ddk/ntddk.h>
#include <intrin.h>

#define MSR_IA32_THERM_STATUS 0x19C
#define MSR_DEVICE_NAME L"\\Device\\NDDrv"
#define MSR_DOSLINK_NAME L"\\DosDevices\\NDDrv"

#define IOCTL_MSR_READ CTL_CODE(FILE_DEVICE_UNKNOWN, 0x800, METHOD_BUFFERED, FILE_ANY_ACCESS)

typedef struct _MSR_REQUEST {
    ULONG msr;
    ULONG cpu;
    ULONG low;
    ULONG high;
} MSR_REQUEST, *PMSR_REQUEST;

DRIVER_UNLOAD     NetdataMsrUnload;
DRIVER_DISPATCH   NetdataMsrCreateClose;
DRIVER_DISPATCH   NetdataMsrDeviceControl;

static UNICODE_STRING g_DeviceName  = RTL_CONSTANT_STRING(MSR_DEVICE_NAME);
static UNICODE_STRING g_DosLinkName = RTL_CONSTANT_STRING(MSR_DOSLINK_NAME);

VOID NetdataMsrUnload(_In_ PDRIVER_OBJECT DriverObject)
{
    IoDeleteSymbolicLink(&g_DosLinkName);

    if (DriverObject->DeviceObject) {
        IoDeleteDevice(DriverObject->DeviceObject);
    }
}

NTSTATUS NetdataMsrCreateClose(_In_ PDEVICE_OBJECT DeviceObject, _Inout_ PIRP Irp)
{
    UNREFERENCED_PARAMETER(DeviceObject);

    Irp->IoStatus.Status = STATUS_SUCCESS;
    Irp->IoStatus.Information = 0;
    IoCompleteRequest(Irp, IO_NO_INCREMENT);
    return STATUS_SUCCESS;
}

NTSTATUS NetdataMsrDeviceControl(_In_ PDEVICE_OBJECT DeviceObject, _Inout_ PIRP Irp)
{
    UNREFERENCED_PARAMETER(DeviceObject);

    PIO_STACK_LOCATION irpSp = IoGetCurrentIrpStackLocation(Irp);
    NTSTATUS status = STATUS_SUCCESS;
    ULONG information = 0;

    if (irpSp->Parameters.DeviceIoControl.IoControlCode == IOCTL_MSR_READ) {
        if (irpSp->Parameters.DeviceIoControl.InputBufferLength < sizeof(MSR_REQUEST) ||
            irpSp->Parameters.DeviceIoControl.OutputBufferLength < sizeof(MSR_REQUEST)) {
            status = STATUS_BUFFER_TOO_SMALL;
        } else {
            PMSR_REQUEST req = (PMSR_REQUEST)Irp->AssociatedIrp.SystemBuffer;

            ULONG maxCpus = KeQueryActiveProcessorCountEx(ALL_PROCESSOR_GROUPS);
            if (req->cpu >= maxCpus) {
                status = STATUS_INVALID_PARAMETER;
            } else {
                KAFFINITY cpuMask = 1ull << req->cpu;
                KAFFINITY oldMask = KeSetSystemAffinityThreadEx(cpuMask);

                ULONGLONG value = __readmsr(req->msr);
                req->low = (ULONG)(value & 0xFFFFFFFF);
                req->high = (ULONG)(value >> 32);
                information = sizeof(MSR_REQUEST);

                KeRevertToUserAffinityThreadEx(oldMask);
            }
        }
    } else {
        status = STATUS_INVALID_DEVICE_REQUEST;
    }

    Irp->IoStatus.Status = status;
    Irp->IoStatus.Information = information;
    IoCompleteRequest(Irp, IO_NO_INCREMENT);

    return status;
}

NTSTATUS DriverEntry(_In_ PDRIVER_OBJECT DriverObject, _In_ PUNICODE_STRING RegistryPath)
{
    UNREFERENCED_PARAMETER(RegistryPath);
    NTSTATUS status;
    PDEVICE_OBJECT deviceObject = NULL;

    status = IoCreateDevice(
            DriverObject,
            0,
            &g_DeviceName,
            FILE_DEVICE_UNKNOWN,
            FILE_DEVICE_SECURE_OPEN,
            FALSE,
            &deviceObject
    );

    if (!NT_SUCCESS(status)) {
        return status;
    }

    status = IoCreateSymbolicLink(&g_DosLinkName, &g_DeviceName);
    if (!NT_SUCCESS(status)) {
        IoDeleteDevice(deviceObject);
        return status;
    }

    deviceObject->Flags |= DO_BUFFERED_IO;

    DriverObject->MajorFunction[IRP_MJ_CREATE]         = NetdataMsrCreateClose;
    DriverObject->MajorFunction[IRP_MJ_CLOSE]          = NetdataMsrCreateClose;
    DriverObject->MajorFunction[IRP_MJ_DEVICE_CONTROL] = NetdataMsrDeviceControl;
    DriverObject->DriverUnload                         = NetdataMsrUnload;

    deviceObject->Flags &= ~DO_DEVICE_INITIALIZING;

    return STATUS_SUCCESS;
}
