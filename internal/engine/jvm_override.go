package engine

import (
	"fmt"
	"math"
	"strings"
)

// GenerateJVMCommandOverride rewrites -Xmx flags in container command or args to 75% of newMemoryBytes.
func GenerateJVMCommandOverride(command []string, args []string, newMemoryBytes float64) ([]string, []string) {
	if newMemoryBytes <= 0 {
		return command, args
	}

	// Calculate new Xmx as 75% of container memory limit
	targetXmxBytes := newMemoryBytes * 0.75
	targetXmxMB := int(math.Floor(targetXmxBytes / (1024 * 1024)))
	newXmxFlag := fmt.Sprintf("-Xmx%dm", targetXmxMB)

	newCommand := make([]string, len(command))
	copy(newCommand, command)
	for i, c := range newCommand {
		if strings.HasPrefix(strings.TrimSpace(c), "-Xmx") {
			newCommand[i] = newXmxFlag
		}
	}

	newArgs := make([]string, len(args))
	copy(newArgs, args)
	for i, a := range newArgs {
		if strings.HasPrefix(strings.TrimSpace(a), "-Xmx") {
			newArgs[i] = newXmxFlag
		}
	}

	return newCommand, newArgs
}
