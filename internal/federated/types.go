package federated

import "time"

const (
	DefaultTrimRatio          = 0.10  // Trim top and bottom 10% outlier deltas
	DefaultMaxDeltaQ          = 5.0   // Outlier rejection cutoff for impossible delta magnitude
	DefaultStabilityThreshold = 0.005 // Maximum shift threshold to declare model STABLE
)

type QStateKey struct {
	CPUBucket   int64 `json:"cpuBucket"`
	MemBucket   int64 `json:"memBucket"`
	CPUReqMilli int64 `json:"cpuReqMilli"`
	MemReqMilli int64 `json:"memReqMilli"`
	JVMBucket   int64 `json:"jvmBucket,omitempty"`
	GPUBucket   int64 `json:"gpuBucket,omitempty"`
}

type QActionKey struct {
	CPUAdjPct int `json:"cpuAdjPct"`
	MemAdjPct int `json:"memAdjPct"`
}

type QTableDelta struct {
	ClusterName string     `json:"clusterName"`
	StateKey    QStateKey  `json:"stateKey"`
	ActionKey   QActionKey `json:"actionKey"`
	DeltaQ      float64    `json:"deltaQ"`
	VisitCount  uint64     `json:"visitCount"`
	Epoch       int64      `json:"epoch"`
	Timestamp   time.Time  `json:"timestamp"`
}

type DeltaBatch struct {
	BatchID     string        `json:"batchId,omitempty"`
	ClusterName string        `json:"clusterName"`
	Epoch       int64         `json:"epoch"`
	Deltas      []QTableDelta `json:"deltas"`
}

type GlobalQTable struct {
	Version              int64                   `json:"version"`
	VersionStr           string                  `json:"versionStr,omitempty"`
	CreatedAt            time.Time               `json:"createdAt"`
	Entries              map[string]GlobalQEntry `json:"entries"`
	IsStable             bool                    `json:"isStable"`
	ConvergenceScore     float64                 `json:"convergenceScore"`
	ModelHash            string                  `json:"modelHash,omitempty"`
	MinSpokeVersion      string                  `json:"minSpokeVersion,omitempty"`
	OutlierRejectedCount uint64                  `json:"outlierRejectedCount"`
}

type GlobalQEntry struct {
	QValue     float64 `json:"qValue"`
	VisitCount uint64  `json:"visitCount"`
}

func EntryKey(s QStateKey, a QActionKey) string {
	return encodeU64(uint64(s.CPUBucket)) + "|" +
		encodeU64(uint64(s.MemBucket)) + "|" +
		encodeU64(uint64(s.CPUReqMilli)) + "|" +
		encodeU64(uint64(s.MemReqMilli)) + "|" +
		encodeU64(uint64(s.JVMBucket)) + "|" +
		encodeU64(uint64(s.GPUBucket)) + "|" +
		encodeI32(a.CPUAdjPct) + "|" +
		encodeI32(a.MemAdjPct)
}

func encodeU64(v uint64) string {
	if v == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func encodeI32(v int) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := encodeU64(uint64(v))
	if neg {
		return "-" + s
	}
	return s
}
