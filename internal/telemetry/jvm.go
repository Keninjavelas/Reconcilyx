package telemetry

import (
	"strconv"
	"strings"
)

// ParseJVMFlags parses container command and args arrays for JVM heap settings (-Xmx, -Xms, -XX:MaxRAMPercentage).
func ParseJVMFlags(command []string, args []string) JVMHeapMetrics {
	var metrics JVMHeapMetrics
	allTokens := append(command, args...)

	for _, token := range allTokens {
		token = strings.TrimSpace(token)

		if strings.HasPrefix(token, "-Xmx") {
			metrics.IsJVMWorkload = true
			metrics.XmxBytes = parseByteQuantity(strings.TrimPrefix(token, "-Xmx"))
		} else if strings.HasPrefix(token, "-Xms") {
			metrics.IsJVMWorkload = true
			metrics.XmsBytes = parseByteQuantity(strings.TrimPrefix(token, "-Xms"))
		} else if strings.Contains(token, "java") || strings.HasSuffix(token, ".jar") {
			metrics.IsJVMWorkload = true
		}
	}

	return metrics
}

// parseByteQuantity converts JVM memory notation (512m, 2g, 1024k, 2048M, 4G) into bytes.
func parseByteQuantity(valStr string) float64 {
	valStr = strings.TrimSpace(valStr)
	if valStr == "" {
		return 0
	}

	unit := valStr[len(valStr)-1]
	numStr := valStr[:len(valStr)-1]

	multiplier := 1.0
	switch unit {
	case 'k', 'K':
		multiplier = 1024.0
	case 'm', 'M':
		multiplier = 1024.0 * 1024.0
	case 'g', 'G':
		multiplier = 1024.0 * 1024.0 * 1024.0
	default:
		// No unit suffix, parse entire string as raw bytes
		numStr = valStr
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}

	return num * multiplier
}
