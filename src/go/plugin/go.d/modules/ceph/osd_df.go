package ceph

type (
	OsdDf struct {
		Nodes   []Node  `json:"nodes"`
		Stray   []any   `json:"stray"`
		Summary Summary `json:"summary"`
	}

	Node struct {
		ID          int64                  `json:"id"`
		DeviceClass string                 `json:"device_class"`
		Name        string                 `json:"name"`
		Type        string                 `json:"type"`
		TypeID      int64                  `json:"type_id"`
		CrushWeight float64                `json:"crush_weight"`
		Depth       int64                  `json:"depth"`
		PoolWeights map[string]interface{} `json:"pool_weights"`
		Reweight    float64                `json:"reweight"`
		KB          int64                  `json:"kb"`
		KBUsed      int64                  `json:"kb_used"`
		KBUsedData  int64                  `json:"kb_used_data"`
		KBUsedOmap  int64                  `json:"kb_used_omap"`
		KBUsedMeta  int64                  `json:"kb_used_meta"`
		KBAvail     int64                  `json:"kb_avail"`
		Utilization float64                `json:"utilization"`
		Var         float64                `json:"var"`
		PGS         int64                  `json:"pgs"`
		Status      string                 `json:"status"`
	}

	Summary struct {
		TotalKB            int64   `json:"total_kb"`
		TotalKBUsed        int64   `json:"total_kb_used"`
		TotalKBUsedData    int64   `json:"total_kb_used_data"`
		TotalKBUsedOmap    int64   `json:"total_kb_used_omap"`
		TotalKBUsedMeta    int64   `json:"total_kb_used_meta"`
		TotalKBAvail       int64   `json:"total_kb_avail"`
		AverageUtilization float64 `json:"average_utilization"`
		MinVar             float64 `json:"min_var"`
		MaxVar             float64 `json:"max_var"`
		Dev                float64 `json:"dev"`
	}
)
