package engine

import (
	"testing"
)

func TestGenerateJVMCommandOverride(t *testing.T) {
	cmd := []string{"java"}
	args := []string{"-Xms128m", "-Xmx1024m", "-jar", "app.jar"}

	// Memory right-sized from 1024Mi to 512Mi (512 * 1024 * 1024 bytes)
	newMemBytes := 512.0 * 1024.0 * 1024.0

	newCmd, newArgs := GenerateJVMCommandOverride(cmd, args, newMemBytes)

	if len(newCmd) != len(cmd) || len(newArgs) != len(args) {
		t.Fatalf("Array lengths mismatch")
	}

	// 75% of 512MB = 384MB (-Xmx384m)
	expectedFlag := "-Xmx384m"

	if newArgs[1] != expectedFlag {
		t.Errorf("Expected -Xmx flag to be updated to %s, got %s", expectedFlag, newArgs[1])
	}

	// Ensure -Xms and -jar were preserved
	if newArgs[0] != "-Xms128m" || newArgs[2] != "-jar" {
		t.Errorf("Other arguments were corrupted: %v", newArgs)
	}
}
