# SPDX-License-Identifier: LGPL-2.1
"""
@package sensors.py
Python Bindings for libsensors3

use the documentation of libsensors for the low level API.
see example.py for high level API usage.

@author: Pavel Rojtberg (http://www.rojtberg.net)
@see: https://github.com/paroj/sensors.py
@copyright: LGPLv2 (same as libsensors) <http://opensource.org/licenses/LGPL-2.1>
"""

from ctypes import *
import ctypes.util

_libc = cdll.LoadLibrary(ctypes.util.find_library("c"))
# see https://github.com/paroj/sensors.py/issues/1
_libc.free.argtypes = [c_void_p]

_hdl = cdll.LoadLibrary(ctypes.util.find_library("sensors"))

version = c_char_p.in_dll(_hdl, "libsensors_version").value.decode("ascii")


class SensorsError(Exception):
    pass


class ErrorWildcards(SensorsError):
    pass


class ErrorNoEntry(SensorsError):
    pass


class ErrorAccessRead(SensorsError, PermissionError):
    pass


class ErrorKernel(SensorError, OSError):
    pass


class ErrorDivZero(SensorError, ZeroDivisionError):
    pass


class ErrorChipName(SensorError):
    pass


class ErrorBusName(SensorError):
    pass


class ErrorParse(SensorError):
    pass


class ErrorAccessWrite(SensorError, PermissionError):
    pass


class ErrorIO(SensorError, IOError):
    pass


class ErrorRecursion(SensorError):
    pass


_ERR_MAP = {
    1: ErrorWildcards,
    2: ErrorNoEntry,
    3: ErrorAccessRead,
    4: ErrorKernel,
    5: ErrorDivZero,
    6: ErrorChipName,
    7: ErrorBusName,
    8: ErrorParse,
    9: ErrorAccessWrite,
    10: ErrorIO,
    11: ErrorRecursion
}


def raise_sensor_error(errno, message=''):
    raise _ERR_MAP[abs(errno)](message)


class bus_id(Structure):
    _fields_ = [("type", c_short),
                ("nr", c_short)]


class chip_name(Structure):
    _fields_ = [("prefix", c_char_p),
                ("bus", bus_id),
                ("addr", c_int),
                ("path", c_char_p)]


class feature(Structure):
    _fields_ = [("name", c_char_p),
                ("number", c_int),
                ("type", c_int)]

    # sensors_feature_type
    IN = 0x00
    FAN = 0x01
    TEMP = 0x02
    POWER = 0x03
    ENERGY = 0x04
    CURR = 0x05
    HUMIDITY = 0x06
    MAX_MAIN = 0x7
    VID = 0x10
    INTRUSION = 0x11
    MAX_OTHER = 0x12
    BEEP_ENABLE = 0x18


class subfeature(Structure):
    _fields_ = [("name", c_char_p),
                ("number", c_int),
                ("type", c_int),
                ("mapping", c_int),
                ("flags", c_uint)]


_hdl.sensors_get_detected_chips.restype = POINTER(chip_name)
_hdl.sensors_get_features.restype = POINTER(feature)
_hdl.sensors_get_all_subfeatures.restype = POINTER(subfeature)
_hdl.sensors_get_label.restype = c_void_p # return pointer instead of str so we can free it
_hdl.sensors_get_adapter_name.restype = c_char_p # docs do not say whether to free this or not
_hdl.sensors_strerror.restype = c_char_p

### RAW API ###
MODE_R = 1
MODE_W = 2
COMPUTE_MAPPING = 4


def init(cfg_file=None):
    file = _libc.fopen(cfg_file.encode("utf-8"), "r") if cfg_file is not None else None

    result = _hdl.sensors_init(file)
    if result != 0:
        raise_sensor_error(result, "sensors_init failed")

    if file is not None:
        _libc.fclose(file)


def cleanup():
    _hdl.sensors_cleanup()


def parse_chip_name(orig_name):
    ret = chip_name()
    err = _hdl.sensors_parse_chip_name(orig_name.encode("utf-8"), byref(ret))

    if err < 0:
        raise_sensor_error(err, strerror(err))

    return ret


def strerror(errnum):
    return _hdl.sensors_strerror(errnum).decode("utf-8")


def free_chip_name(chip):
    _hdl.sensors_free_chip_name(byref(chip))


def get_detected_chips(match, nr):
    """
    @return: (chip, next nr to query)
    """
    _nr = c_int(nr)

    if match is not None:
        match = byref(match)

    chip = _hdl.sensors_get_detected_chips(match, byref(_nr))
    chip = chip.contents if bool(chip) else None
    return chip, _nr.value


def chip_snprintf_name(chip, buffer_size=200):
    """
    @param buffer_size defaults to the size used in the sensors utility
    """
    ret = create_string_buffer(buffer_size)
    err = _hdl.sensors_snprintf_chip_name(ret, buffer_size, byref(chip))

    if err < 0:
        raise_sensor_error(err, strerror(err))

    return ret.value.decode("utf-8")


def do_chip_sets(chip):
    """
    @attention this function was not tested
    """
    err = _hdl.sensors_do_chip_sets(byref(chip))
    if err < 0:
        raise_sensor_error(err, strerror(err))


def get_adapter_name(bus):
    return _hdl.sensors_get_adapter_name(byref(bus)).decode("utf-8")


def get_features(chip, nr):
    """
    @return: (feature, next nr to query)
    """
    _nr = c_int(nr)
    feature = _hdl.sensors_get_features(byref(chip), byref(_nr))
    feature = feature.contents if bool(feature) else None
    return feature, _nr.value


def get_label(chip, feature):
    ptr = _hdl.sensors_get_label(byref(chip), byref(feature))
    val = cast(ptr, c_char_p).value.decode("utf-8")
    _libc.free(ptr)
    return val


def get_all_subfeatures(chip, feature, nr):
    """
    @return: (subfeature, next nr to query)
    """
    _nr = c_int(nr)
    subfeature = _hdl.sensors_get_all_subfeatures(byref(chip), byref(feature), byref(_nr))
    subfeature = subfeature.contents if bool(subfeature) else None
    return subfeature, _nr.value


def get_value(chip, subfeature_nr):
    val = c_double()
    err = _hdl.sensors_get_value(byref(chip), subfeature_nr, byref(val))
    if err < 0:
        raise_sensor_error(err, strerror(err))
    return val.value


def set_value(chip, subfeature_nr, value):
    """
    @attention this function was not tested
    """
    val = c_double(value)
    err = _hdl.sensors_set_value(byref(chip), subfeature_nr, byref(val))
    if err < 0:
        raise_sensor_error(err, strerror(err))


### Convenience API ###
class ChipIterator:
    def __init__(self, match=None):
        self.match = parse_chip_name(match) if match is not None else None
        self.nr = 0

    def __iter__(self):
        return self

    def __next__(self):
        chip, self.nr = get_detected_chips(self.match, self.nr)

        if chip is None:
            raise StopIteration

        return chip

    def __del__(self):
        if self.match is not None:
            free_chip_name(self.match)

    def next(self): # python2 compability
        return self.__next__()


class FeatureIterator:
    def __init__(self, chip):
        self.chip = chip
        self.nr = 0

    def __iter__(self):
        return self

    def __next__(self):
        feature, self.nr = get_features(self.chip, self.nr)

        if feature is None:
            raise StopIteration

        return feature

    def next(self): # python2 compability
        return self.__next__()


class SubFeatureIterator:
    def __init__(self, chip, feature):
        self.chip = chip
        self.feature = feature
        self.nr = 0

    def __iter__(self):
        return self

    def __next__(self):
        subfeature, self.nr = get_all_subfeatures(self.chip, self.feature, self.nr)

        if subfeature is None:
            raise StopIteration

        return subfeature

    def next(self): # python2 compability
        return self.__next__()
