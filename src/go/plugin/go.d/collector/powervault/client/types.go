// SPDX-License-Identifier: GPL-3.0-or-later

package client

// StatusResponse is the common status envelope in every MCI API response.
type StatusResponse struct {
	ResponseType    string `json:"response-type"`
	Response        string `json:"response"`
	ReturnCode      int    `json:"return-code"`
	ResponseTypeNum int    `json:"response-type-numeric"`
}

// SystemInfo from /api/show/system.
type SystemInfo struct {
	SystemName     string `json:"system-name"`
	ProductID      string `json:"product-id"`
	HealthNumeric  int    `json:"health-numeric"`
	Health         string `json:"health"`
	MidplaneSerial string `json:"midplane-serial-number"`
	VendorName     string `json:"vendor-name"`
	PlatformBrand  string `json:"platform-brand"`
	EnclosureCount int    `json:"enclosure-count"`
	SystemLocation string `json:"system-location"`
	SystemContact  string `json:"system-contact"`
	SCProductID    string `json:"scsi-product-id"`
	CurrentNodeWWN string `json:"current-node-wwn"`
}

// Controller from /api/show/controllers.
type Controller struct {
	DurableID        string `json:"durable-id"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	Health           string `json:"health"`
	HealthNumeric    int    `json:"health-numeric"`
	SerialNumber     string `json:"serial-number"`
	RedundancyStatus string `json:"redundancy-status"`
}

// Drive from /api/show/disks (response key: "drives").
type Drive struct {
	DurableID       string `json:"durable-id"`
	Location        string `json:"location"`
	SerialNumber    string `json:"serial-number"`
	Model           string `json:"model"`
	Vendor          string `json:"vendor"`
	Description     string `json:"description"`
	Architecture    string `json:"architecture"` // HDD or SSD
	Interface       string `json:"interface"`    // SAS, SATA, NVMe
	Size            string `json:"size"`
	SizeNumeric     int64  `json:"size-numeric"` // in 512-byte blocks
	Health          string `json:"health"`
	HealthNumeric   int    `json:"health-numeric"`
	Status          string `json:"status"`
	TempNumeric     int    `json:"temperature-numeric"`
	SSDLifeLeft     int    `json:"ssd-life-left-numeric"` // 255 = N/A (HDD)
	PowerOnHours    int    `json:"power-on-hours"`
	Usage           string `json:"usage"`
	UsageNumeric    int    `json:"usage-numeric"` // 0=AVAIL, 3=GLOBAL SP, 7=FAILED, 8=UNUSABLE, 9=VIRTUAL POOL
	EnclosureID     int    `json:"enclosure-id"`
	Slot            int    `json:"slot"`
	StorageTier     string `json:"storage-tier"`
	DiskGroup       string `json:"disk-group"`
	StoragePoolName string `json:"storage-pool-name"`
}

// Fan from /api/show/fans (response key: "fan").
type Fan struct {
	DurableID     string `json:"durable-id"`
	Name          string `json:"name"`
	Location      string `json:"location"`
	Health        string `json:"health"`
	HealthNumeric int    `json:"health-numeric"`
	Status        string `json:"status"`
	Speed         int    `json:"speed"` // RPM
}

// PowerSupply from /api/show/power-supplies.
type PowerSupply struct {
	DurableID     string `json:"durable-id"`
	Name          string `json:"name"`
	Location      string `json:"location"`
	Health        string `json:"health"`
	HealthNumeric int    `json:"health-numeric"`
	Status        string `json:"status"`
	SerialNumber  string `json:"serial-number"`
	Model         string `json:"model"`
	Position      string `json:"position"`
}

// Sensor from /api/show/sensor-status (response key: "sensors").
type Sensor struct {
	DurableID     string `json:"durable-id"`
	SensorName    string `json:"sensor-name"`
	SensorType    string `json:"sensor-type"` // Temperature, Voltage, Current, Charge Capacity
	Value         string `json:"value"`       // e.g. "32 C", "3.3 V", "95%"
	Status        string `json:"status"`
	StatusNumeric int    `json:"status-numeric"` // DIFFERENT mapping: 0=Unsupported,1=OK,2=Critical,3=Warning,...
}

// FRU from /api/show/frus (response key: "enclosure-fru").
type FRU struct {
	Name         string `json:"name"`
	FRULocation  string `json:"fru-location"`
	FRUStatus    string `json:"fru-status"`
	FRUStatusNum int    `json:"fru-status-numeric"` // 0=OK, 1=Degraded, 2=Fault, 3=Unknown, 5=N/A→OK
	PartNumber   string `json:"part-number"`
}

// Volume from /api/show/volumes (response key: "volumes").
type Volume struct {
	DurableID     string `json:"durable-id"`
	VolumeName    string `json:"volume-name"`
	Health        string `json:"health"`
	HealthNumeric int    `json:"health-numeric"`
	TotalSize     string `json:"total-size"`
	OwnerNumeric  int    `json:"owner-numeric"`
}

// Pool from /api/show/pools.
type Pool struct {
	Name          string `json:"name"`
	Health        string `json:"health"`
	HealthNumeric int    `json:"health-numeric"`
	TotalSize     string `json:"total-size"`
	TotalAvail    string `json:"total-avail"`
	// Numeric values in 512-byte blocks
	TotalSizeNumeric  int64 `json:"total-size-numeric"`
	TotalAvailNumeric int64 `json:"total-avail-numeric"`
}

// Port from /api/show/ports.
type Port struct {
	DurableID     string `json:"durable-id"`
	Port          string `json:"port"`
	Health        string `json:"health"`
	HealthNumeric int    `json:"health-numeric"` // 0-3 + 4=Not Available
	Status        string `json:"status"`
	StatusNumeric int    `json:"status-numeric"` // 0=up, 1=down, 2=notInstalled
	ActualSpeed   string `json:"actual-speed"`
	TargetID      string `json:"target-id"`
}

// ControllerStats from /api/show/controller-statistics.
type ControllerStats struct {
	DurableID        string  `json:"durable-id"`
	IOPS             int64   `json:"iops"`
	BytesPerSecond   int64   `json:"bytes-per-second-numeric"`
	CPULoad          float64 `json:"cpu-load"`
	DataRead         int64   `json:"data-read-numeric"`
	DataWritten      int64   `json:"data-written-numeric"`
	NumReads         int64   `json:"number-of-reads"`
	NumWrites        int64   `json:"number-of-writes"`
	WriteCacheUsed   int     `json:"write-cache-used"`
	WriteCacheHits   int64   `json:"write-cache-hits"`
	WriteCacheMisses int64   `json:"write-cache-misses"`
	ReadCacheHits    int64   `json:"read-cache-hits"`
	ReadCacheMisses  int64   `json:"read-cache-misses"`
	ForwardedCmds    int64   `json:"num-forwarded-cmds"`
}

// VolumeStats from /api/show/volume-statistics.
type VolumeStats struct {
	VolumeName        string `json:"volume-name"`
	IOPS              int64  `json:"iops"`
	BytesPerSecond    int64  `json:"bytes-per-second-numeric"`
	DataRead          int64  `json:"data-read-numeric"`
	DataWritten       int64  `json:"data-written-numeric"`
	NumReads          int64  `json:"number-of-reads"`
	NumWrites         int64  `json:"number-of-writes"`
	WriteCachePercent int    `json:"write-cache-percent"`
	WriteCacheHits    int64  `json:"write-cache-hits"`
	WriteCacheMisses  int64  `json:"write-cache-misses"`
	ReadCacheHits     int64  `json:"read-cache-hits"`
	ReadCacheMisses   int64  `json:"read-cache-misses"`
	TierSSD           int    `json:"percent-tier-ssd"`
	TierSAS           int    `json:"percent-tier-sas"`
	TierSATA          int    `json:"percent-tier-sata"`
}

// PortStats from /api/show/host-port-statistics.
type PortStats struct {
	DurableID   string `json:"durable-id"`
	NumReads    int64  `json:"number-of-reads"`
	NumWrites   int64  `json:"number-of-writes"`
	DataRead    int64  `json:"data-read-numeric"`
	DataWritten int64  `json:"data-write-numeric"` // note: "data-write" not "data-written"
}

// PhyStats from /api/show/host-phy-statistics (response key: "sas-host-phy-statistics").
type PhyStats struct {
	Port            string `json:"port"`
	Phy             string `json:"phy"`
	DisparityErrors int64  `json:"disparity-errors"`
	LostDwords      int64  `json:"lost-dwords"`
	InvalidDwords   int64  `json:"invalid-dwords"`
}
