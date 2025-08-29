// SPDX-License-Identifier: GPL-3.0-or-later

#include <ddk/ntddk.h>

#define MSR_IA32_THERM_STATUS 0x19C
#define MSR_DEVICE_NAME L"\\Device\\NDDrv"
#define MSR_DOSLINK_NAME L"\\DosDevices\\NDDrv"

#define IOCTL_MSR_READ   CTL_CODE(FILE_DEVICE_UNKNOWN, 0x800, METHOD_BUFFERED, FILE_ANY_ACCESS)
#define IOCTL_MSR_THERM  CTL_CODE(FILE_DEVICE_UNKNOWN, 0x801, METHOD_BUFFERED, FILE_ANY_ACCESS)

// Payloads
typedef struct _MSR_READ_INPUT {
    ULONG Reg;           // MSR index to read (e.g., 0x19C)
} MSR_READ_INPUT, *PMSR_READ_INPUT;

typedef struct _MSR_READ_OUTPUT {
    ULONGLONG Value;     // Raw 64-bit MSR value
} MSR_READ_OUTPUT, *PMSR_READ_OUTPUT;

typedef struct _MSR_THERM_OUTPUT {
    ULONG DeltaToTjMax;  // From IA32_THERM_STATUS[22:16]
} MSR_THERM_OUTPUT, *PMSR_THERM_OUTPUT;

DRIVER_UNLOAD     NetdataMsrUnload;
DRIVER_DISPATCH   NetdataMsrCreateClose;
DRIVER_DISPATCH   NetdataMsrDeviceControl;

extern ULONGLONG __readmsr(ULONG reg);

static UNICODE_STRING g_DeviceName  = RTL_CONSTANT_STRING(MSR_DEVICE_NAME);
static UNICODE_STRING g_DosLinkName = RTL_CONSTANT_STRING(MSR_DOSLINK_NAME);

VOID NetdataMsrUnload(_In_ PDRIVER_OBJECT DriverObject)
{
    UNREFERENCED_PARAMETER(DriverObject);
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

NTSTATUS SafeReadMsr(_In_ ULONG reg, _Out_ PULONGLONG outVal)
{
    *outVal = __readmsr(reg);
    return STATUS_SUCCESS;
}

NTSTATUS NetdataMsrDeviceControl(_In_ PDEVICE_OBJECT DeviceObject, _Inout_ PIRP Irp)
{
    UNREFERENCED_PARAMETER(DeviceObject);
    PIO_STACK_LOCATION irpSp = IoGetCurrentIrpStackLocation(Irp);

    NTSTATUS status = STATUS_INVALID_DEVICE_REQUEST;
    ULONG_PTR info = 0;

    if (irpSp->Parameters.DeviceIoControl.IoControlCode == IOCTL_MSR_READ) {
        // Buffered I/O: both input and output are in SystemBuffer
        if (irpSp->Parameters.DeviceIoControl.InputBufferLength  < sizeof(MSR_READ_INPUT) ||
            irpSp->Parameters.DeviceIoControl.OutputBufferLength < sizeof(MSR_READ_OUTPUT)) {
            status = STATUS_BUFFER_TOO_SMALL;
        } else {
            PMSR_READ_INPUT in  = (PMSR_READ_INPUT)Irp->AssociatedIrp.SystemBuffer;
            PMSR_READ_OUTPUT out = (PMSR_READ_OUTPUT)Irp->AssociatedIrp.SystemBuffer;

            out->Value = __readmsr(in->Reg);
            info = sizeof(MSR_READ_OUTPUT);
        }
    }
    else if (irpSp->Parameters.DeviceIoControl.IoControlCode == IOCTL_MSR_THERM) {
        if (irpSp->Parameters.DeviceIoControl.OutputBufferLength < sizeof(MSR_THERM_OUTPUT)) {
            status = STATUS_BUFFER_TOO_SMALL;
        } else {
            PMSR_THERM_OUTPUT out = (PMSR_THERM_OUTPUT)Irp->AssociatedIrp.SystemBuffer;
            ULONGLONG val = __readmsr(MSR_IA32_THERM_STATUS);
            // IA32_THERM_STATUS bits [22:16] = Digital Readout = Delta to TjMax
            out->DeltaToTjMax = (ULONG)((val >> 16) & 0x7F);
            info = sizeof(MSR_THERM_OUTPUT);
        }
    }

    Irp->IoStatus.Status = status;
    Irp->IoStatus.Information = info;
    IoCompleteRequest(Irp, IO_NO_INCREMENT);
    return status;
}

NTSTATUS DriverEntry(_In_ PDRIVER_OBJECT DriverObject, _In_ PUNICODE_STRING RegistryPath)
{
    UNREFERENCED_PARAMETER(RegistryPath);

    PDEVICE_OBJECT deviceObject = NULL;
    NTSTATUS status;

    status = IoCreateDevice(
        DriverObject,
        0,                      // no device extension
        &g_DeviceName,
        FILE_DEVICE_UNKNOWN,
        FILE_DEVICE_SECURE_OPEN,
        FALSE,
        &deviceObject
    );
    if (!NT_SUCCESS(status)) {
        return status;
    }

    // Use buffered I/O for simple IOCTL marshaling
    deviceObject->Flags |= DO_BUFFERED_IO;

    status = IoCreateSymbolicLink(&g_DosLinkName, &g_DeviceName);
    if (!NT_SUCCESS(status)) {
        IoDeleteDevice(deviceObject);
        return status;
    }

    // Dispatch routines
    DriverObject->MajorFunction[IRP_MJ_CREATE]         = NetdataMsrCreateClose;
    DriverObject->MajorFunction[IRP_MJ_CLOSE]          = NetdataMsrCreateClose;
    DriverObject->MajorFunction[IRP_MJ_DEVICE_CONTROL] = NetdataMsrDeviceControl;
    DriverObject->DriverUnload                         = NetdataMsrUnload;

    deviceObject->Flags &= ~DO_DEVICE_INITIALIZING;
    return STATUS_SUCCESS;
}
