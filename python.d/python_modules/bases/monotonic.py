# thx to https://stackoverflow.com/a/1205762
import ctypes
# import os

CLOCK_MONOTONIC = 1  # see <linux/time.h>


class _Timespec(ctypes.Structure):
    _fields_ = [
        ('tv_sec', ctypes.c_long),
        ('tv_nsec', ctypes.c_long)
    ]


_librt = ctypes.CDLL('librt.so.1', use_errno=True)
_clock_gettime = _librt.clock_gettime
_clock_gettime.argtypes = [ctypes.c_int, ctypes.POINTER(_Timespec)]


def time():
    t = _Timespec()
    if _clock_gettime(CLOCK_MONOTONIC, ctypes.pointer(t)) != 0:
        # errno_ = ctypes.get_errno()
        # raise OSError(errno_, os.strerror(errno_))
        return None
    return t.tv_sec + t.tv_nsec * 1e-9


AVAILABLE = bool(time())
