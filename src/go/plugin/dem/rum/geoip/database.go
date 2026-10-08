// SPDX-License-Identifier: GPL-3.0-or-later

package geoip

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"unsafe"

	"github.com/oschwald/maxminddb-golang"
)

var errMappedFault = errors.New("mapped database read fault")
var errUnsupportedDatabase = errors.New("unsupported geographic database type")

type mapping struct {
	data    []byte
	release func() error
}

func (m *mapping) close() error {
	if m.release == nil {
		return nil
	}
	err := m.release()
	m.release = nil
	m.data = nil
	return err
}

// Only faults inside this owned mapping belong to the optional database boundary.
// An unrelated runtime/programming panic must keep its normal behavior.
func (m *mapping) read(fn func() error) (err error) {
	previous := debug.SetPanicOnFault(true)
	defer debug.SetPanicOnFault(previous)
	defer runtime.KeepAlive(m)
	defer func() {
		if value := recover(); value != nil {
			fault, ok := value.(interface {
				runtime.Error
				Addr() uintptr
			})
			base := uintptr(unsafe.Pointer(unsafe.SliceData(m.data)))
			if ok && fault.Addr() >= base && fault.Addr()-base < uintptr(len(m.data)) {
				err = errMappedFault
				return
			}
			panic(value)
		}
	}()
	return fn()
}

type database struct {
	reader  *maxminddb.Reader
	mapping *mapping
}

func openDatabase(path string) (*database, os.FileInfo, error) {
	f, err := openFile(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || int64(int(info.Size())) != info.Size() {
		return nil, nil, errors.New("database must be a nonempty regular file of addressable size")
	}
	region, err := mapFile(f, int(info.Size()))
	if err != nil {
		return nil, nil, err
	}
	db, err := decodeDatabase(region)
	return db, info, err
}

// Ownership transfers on entry, including unsuccessful or panicking construction.
func decodeDatabase(region *mapping) (db *database, err error) {
	accepted := false
	defer func() {
		if !accepted {
			_ = region.close()
		}
	}()
	var reader *maxminddb.Reader
	err = region.read(func() error {
		var decodeErr error
		reader, decodeErr = maxminddb.FromBytes(region.data)
		return decodeErr
	})
	if err != nil {
		return nil, err
	}
	switch reader.Metadata.DatabaseType {
	case "Netdata-Topology-GEO",
		"GeoLite2-City",
		"GeoLite2-Country",
		"GeoIP2-City",
		"GeoIP2-Country",
		"DBIP-City-Lite",
		"DBIP-Country-Lite":
	default:
		_ = reader.Close()
		return nil, fmt.Errorf("%w: %s", errUnsupportedDatabase, reader.Metadata.DatabaseType)
	}
	accepted = true
	return &database{
		reader:  reader,
		mapping: region,
	}, nil
}

func (d *database) lookup(ip net.IP, record *geoRecord) error {
	return d.mapping.read(func() error { return d.reader.Lookup(ip, record) })
}

func (d *database) close() {
	_ = d.reader.Close()
	_ = d.mapping.close()
}
