package telemetry

import (
	"testing"
)

func TestParseJVMFlags(t *testing.T) {
	tests := []struct {
		name     string
		command  []string
		args     []string
		wantJVM  bool
		wantXmx  float64
		wantXms  float64
	}{
		{
			name:    "Standard java jar execution with -Xmx512m and -Xms128m",
			command: []string{"java"},
			args:    []string{"-Xms128m", "-Xmx512m", "-jar", "app.jar"},
			wantJVM: true,
			wantXmx: 512 * 1024 * 1024,
			wantXms: 128 * 1024 * 1024,
		},
		{
			name:    "Gigabyte notation -Xmx2g",
			command: []string{"/usr/bin/java"},
			args:    []string{"-Xmx2g", "-jar", "server.jar"},
			wantJVM: true,
			wantXmx: 2 * 1024 * 1024 * 1024,
			wantXms: 0,
		},
		{
			name:    "Non-JVM workload",
			command: []string{"/app/server"},
			args:    []string{"--port=8080"},
			wantJVM: false,
			wantXmx: 0,
			wantXms: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseJVMFlags(tt.command, tt.args)
			if got.IsJVMWorkload != tt.wantJVM {
				t.Errorf("IsJVMWorkload = %v, want %v", got.IsJVMWorkload, tt.wantJVM)
			}
			if got.XmxBytes != tt.wantXmx {
				t.Errorf("XmxBytes = %v, want %v", got.XmxBytes, tt.wantXmx)
			}
			if got.XmsBytes != tt.wantXms {
				t.Errorf("XmsBytes = %v, want %v", got.XmsBytes, tt.wantXms)
			}
		})
	}
}
