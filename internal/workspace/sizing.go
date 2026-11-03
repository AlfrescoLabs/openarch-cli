package workspace

// Sizing holds the memory settings of the services.
type Sizing struct {
	// Budget is the memory the sizing was computed for, in bytes.
	Budget int64 `json:"budget"`
	// OpenArchHeap and OpenSearchHeap are JVM sizes such as 1g or 512m.
	OpenArchHeap   string `json:"openarchHeap"`
	OpenSearchHeap string `json:"opensearchHeap"`
	// MongoCacheGB is the WiredTiger cache size in GB.
	MongoCacheGB string `json:"mongoCacheGB"`
}

// FallbackMemory is the budget used with memory auto when Docker does not report its memory.
const FallbackMemory int64 = 4 << 30

// SizingFor returns the memory settings for a budget in bytes. The steps are
// estimates: the services need about 3 GiB to start, and more memory mostly
// helps OpenSearch and the OpenArch JVM.
func SizingFor(budget int64) Sizing {
	switch {
	case budget < 6<<30:
		return Sizing{Budget: budget, OpenArchHeap: "1g", OpenSearchHeap: "512m", MongoCacheGB: "0.25"}
	case budget < 12<<30:
		return Sizing{Budget: budget, OpenArchHeap: "2g", OpenSearchHeap: "1g", MongoCacheGB: "0.5"}
	default:
		return Sizing{Budget: budget, OpenArchHeap: "4g", OpenSearchHeap: "2g", MongoCacheGB: "1"}
	}
}
