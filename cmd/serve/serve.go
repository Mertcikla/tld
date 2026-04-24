package serve

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/localserver"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func defaultServeRunE(cmd *cobra.Command, args []string) error {
	_ = workspace.EnsureGlobalConfig()

	foreground, _ := cmd.Flags().GetBool("foreground")
	openBrowser, _ := cmd.Flags().GetBool("open")
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetString("port")
	dataDirFlag, _ := cmd.Flags().GetString("data-dir")

	cfg, _ := workspace.LoadGlobalConfig()
	dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
	if err != nil {
		return err
	}

	if foreground {
		return runForeground(cmd, host, port, dataDir, openBrowser)
	}
	return runBackground(cmd, host, port, dataDir, openBrowser)
}

func runForeground(cmd *cobra.Command, host, port, dataDir string, openBrowser bool) error {
	opts := resolveServeOptions(host, port)

	app, err := localserver.Bootstrap(dataDir, opts)
	if err != nil {
		return err
	}

	PrintLogo(cmd.OutOrStdout())
	url := "http://" + app.Addr
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Webapp available at %s\n", url)

	if openBrowser {
		_ = cmdutil.OpenBrowser(url)
	}

	srv := &http.Server{Addr: app.Addr, Handler: app.Handler}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runBackground(cmd *cobra.Command, host, port, dataDir string, openBrowser bool) error {
	pidPath := localserver.PIDPath(dataDir)
	opts := resolveServeOptions(host, port)
	addr := localserver.ResolveAddr(opts)
	url := "http://" + addr

	if pid, err := localserver.ReadPID(pidPath); err == nil && localserver.IsRunning(pid) {
		PrintLogo(cmd.OutOrStdout())
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Server already running (pid %d)\nWebapp available at %s\n", pid, url)
		if openBrowser {
			_ = cmdutil.OpenBrowser(url)
		}
		return nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	fwdArgs := []string{"serve", "--foreground"}
	if host != "" {
		fwdArgs = append(fwdArgs, "--host", host)
	}
	if port != "" {
		fwdArgs = append(fwdArgs, "--port", port)
	}
	// Always pass resolved dataDir to foreground child
	fwdArgs = append(fwdArgs, "--data-dir", dataDir)

	lf, err := os.OpenFile(localserver.LogPath(dataDir), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = lf.Close() }()

	child := exec.Command(exe, fwdArgs...)
	child.Stdout = lf
	child.Stderr = lf
	child.SysProcAttr = getSysProcAttr()

	if err := child.Start(); err != nil {
		return fmt.Errorf("start server process: %w", err)
	}

	if err := localserver.WritePID(pidPath, child.Process.Pid); err != nil {
		_ = child.Process.Kill()
		return fmt.Errorf("write pid file: %w", err)
	}

	if err := waitReady(url+"/api/ready", 10*time.Second); err != nil {
		_ = child.Process.Kill()
		_ = os.Remove(pidPath)
		return fmt.Errorf("server did not become ready: %w\nCheck logs: %s", err, localserver.LogPath(dataDir))
	}

	if !localserver.IsRunning(child.Process.Pid) {
		_ = os.Remove(pidPath)
		return fmt.Errorf("server process exited immediately; check logs: %s", localserver.LogPath(dataDir))
	}

	PrintLogo(cmd.OutOrStdout())
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Webapp available at %s\n", url)

	if openBrowser {
		_ = cmdutil.OpenBrowser(url)
	}
	return nil
}

func waitReady(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}

func resolveServeOptions(flagHost, flagPort string) localserver.ServeOptions {
	cfg, _ := workspace.LoadGlobalConfig()
	host := cfg.Serve.Host
	port := cfg.Serve.Port
	if flagHost != "" {
		host = flagHost
	}
	if flagPort != "" {
		port = flagPort
	}
	return localserver.ServeOptions{Host: host, Port: port}
}

func NewServeCmd(runE func(*cobra.Command, []string) error) *cobra.Command {
	if runE == nil {
		runE = defaultServeRunE
	}

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the local tlDiagram web server",
		Long: `Start the tlDiagram web server as a background process.

Connection details are printed once the server is ready.
Use 'tld stop' to shut it down.

Host and port can be set via flags, the global config file
(~/.config/tldiagram/tld.yaml under serve.host / serve.port),
or the TLD_ADDR / PORT environment variables.`,
		RunE: runE,
	}

	cmd.Flags().String("host", "", "host address to bind (overrides config and env)")
	cmd.Flags().String("port", "", "port to listen on (overrides config and env)")
	cmd.Flags().String("data-dir", "", "directory for database and logs (overrides config and env)")
	cmd.Flags().Bool("open", false, "open the browser automatically")
	cmd.Flags().Bool("foreground", false, "run server in foreground (internal)")
	_ = cmd.Flags().MarkHidden("foreground")

	return cmd
}
