package tools

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

const outputLimit = 64 << 10
const truncatedOutput = "[earlier tool output truncated]\n"

// Run bounds diagnostic memory and waits for inherited output pipes. On Unix,
// cancellation also kills descendants in the tool's process group.
func Run(ctx context.Context, path string, args []string, dir string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	cleanup := configureProcess(cmd)
	defer cleanup()
	var output tailOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	err := cmd.Run()
	if ctx.Err() != nil {
		err = errors.Join(ctx.Err(), err)
	}
	return output.bytes(), err
}

type tailOutput struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (w *tailOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	if len(w.data)+n > outputLimit {
		w.truncated = true
		if n >= outputLimit {
			w.data = append(w.data[:0], p[n-outputLimit:]...)
			return n, nil
		}
		keep := outputLimit - n
		copy(w.data, w.data[len(w.data)-keep:])
		w.data = w.data[:keep]
	}
	w.data = append(w.data, p...)
	return n, nil
}

func (w *tailOutput) bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.truncated {
		return append([]byte(truncatedOutput), w.data...)
	}
	return append([]byte(nil), w.data...)
}
