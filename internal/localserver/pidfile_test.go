package localserver

import (
	"os"
	"testing"
)

func TestIsRunningDetectsCurrentProcess(t *testing.T) {
	if !IsRunning(os.Getpid()) {
		t.Fatal("current process should be reported as running")
	}
}

func TestIsRunningRejectsInvalidPID(t *testing.T) {
	if IsRunning(0) {
		t.Fatal("pid 0 should not be reported as running")
	}
}
