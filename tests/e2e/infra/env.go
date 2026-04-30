package infra

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type TldEnv struct {
	RootDir string
	BinPath string
	DataDir string
	Port    int

	cmd *exec.Cmd
	wg  sync.WaitGroup
}

func NewTldEnv(rootDir string, port int) *TldEnv {
	return &TldEnv{
		RootDir: rootDir,
		DataDir: filepath.Join(rootDir, "tld-data"),
		Port:    port,
	}
}

func (e *TldEnv) Build(srcDir string) error {
	e.BinPath = filepath.Join(e.RootDir, "tld-bin")
	cmd := exec.Command("go", "build", "-o", e.BinPath, "./cmd/tld")
	cmd.Dir = srcDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build failed: %v\n%s", err, string(out))
	}
	return nil
}

func (e *TldEnv) Watch(repoPath string) error {
	e.cmd = exec.Command(e.BinPath, "watch", repoPath,
		"--data-dir", e.DataDir,
		"--port", fmt.Sprintf("%d", e.Port),
		"--verbose",
		"--no-serve=false", // We want the server to run
		"--open=false",
		"--embedding-provider", "none",
	)
	
	// Ensure we don't inherit env that might point to real data
	e.cmd.Env = append(os.Environ(), "TLD_DATA_DIR="+e.DataDir)

	stdout, _ := e.cmd.StdoutPipe()
	stderr, _ := e.cmd.StderrPipe()

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for scanner.Scan() {
			fmt.Printf("[tld-watch:%d] %s\n", e.Port, scanner.Text())
		}
	}()

	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start watch: %v", err)
	}

	return nil
}

func (e *TldEnv) Stop() error {
	if e.cmd == nil || e.cmd.Process == nil {
		return nil
	}
	
	// Try graceful stop via signal
	_ = e.cmd.Process.Signal(os.Interrupt)
	
	done := make(chan error, 1)
	go func() {
		done <- e.cmd.Wait()
	}()

	select {
	case <-done:
		// Stopped
	case <-time.After(5 * time.Second):
		_ = e.cmd.Process.Kill()
	}

	e.wg.Wait()
	return nil
}

func (e *TldEnv) Cleanup() {
	_ = e.Stop()
	_ = os.RemoveAll(e.RootDir)
}
