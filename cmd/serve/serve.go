package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/localserver"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

const (
	backgroundReadyTimeout = 30 * time.Second
	readyRequestTimeout    = 10 * time.Second
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
	started := time.Now()
	opts := resolveServeOptions(host, port)

	app, err := localserver.Bootstrap(dataDir, opts)
	if err != nil {
		return err
	}

	PrintLogo(cmd.OutOrStdout())
	url := "http://" + app.Addr
	printServeInfo(cmd.OutOrStdout(), url, serveStatus{
		Mode:            "foreground",
		InitializedData: app.InitializedData,
		Resources:       app.Resources,
		BindAddr:        app.Addr,
		Startup:         time.Since(started),
		DBPath:          app.DBPath,
	})

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
	started := time.Now()
	pidPath := localserver.PIDPath(dataDir)
	opts := resolveServeOptions(host, port)
	addr := localserver.ResolveAddr(opts)
	url := "http://" + addr
	initializedData := databaseWillBeInitialized(dataDir)

	if pid, err := localserver.ReadPID(pidPath); err == nil && localserver.IsRunning(pid) {
		PrintLogo(cmd.OutOrStdout())
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Server already running (pid %d)\n", pid)
		ready, _ := getReady(url + "/api/ready")
		printServeInfo(cmd.OutOrStdout(), url, serveStatus{
			Mode:            "background",
			InitializedData: initializedData,
			Resources:       readyResources(ready),
			PID:             intPtr(pid),
			BindAddr:        addr,
			Startup:         0,
			DBPath:          localserver.DatabasePath(dataDir),
		})
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

	ready, err := waitReady(url+"/api/ready", backgroundReadyTimeout)
	if err != nil {
		_ = child.Process.Kill()
		_ = os.Remove(pidPath)
		return fmt.Errorf("server did not become ready: %w\nCheck logs: %s", err, localserver.LogPath(dataDir))
	}

	if !localserver.IsRunning(child.Process.Pid) {
		_ = os.Remove(pidPath)
		return fmt.Errorf("server process exited immediately; check logs: %s", localserver.LogPath(dataDir))
	}

	PrintLogo(cmd.OutOrStdout())
	printServeInfo(cmd.OutOrStdout(), url, serveStatus{
		Mode:            "background",
		InitializedData: initializedData,
		Resources:       readyResources(ready),
		PID:             intPtr(child.Process.Pid),
		BindAddr:        addr,
		Startup:         time.Since(started),
		DBPath:          localserver.DatabasePath(dataDir),
	})

	if openBrowser {
		_ = cmdutil.OpenBrowser(url)
	}
	return nil
}

type serveStatus struct {
	Mode            string
	PID             *int
	BindAddr        string
	InitializedData bool
	Resources       localserver.ResourceCounts
	Startup         time.Duration
	DBPath          string
}

func printServeInfo(out io.Writer, url string, status serveStatus) {
	cfgPath, _ := workspace.ConfigPath()
	_, _ = fmt.Fprintf(out, "Mode:                %s\n", printableMode(status.Mode))
	if status.PID != nil {
		_, _ = fmt.Fprintf(out, "PID:                 %d\n", *status.PID)
	}
	_, _ = fmt.Fprintf(out, "Server status:       %s\n", dataStatus(status.InitializedData))
	_, _ = fmt.Fprintf(out, "Bind address:        %s\n", status.BindAddr)
	if !status.InitializedData {
		_, _ = fmt.Fprintf(out, "Resource counts:     %d views, %d elements, %d connectors\n", status.Resources.Views, status.Resources.Elements, status.Resources.Connectors)
	}
	if status.Startup > 0 {
		_, _ = fmt.Fprintf(out, "Ready in:            %s\n", status.Startup.Round(time.Millisecond))
	}
	_, _ = fmt.Fprintf(out, "DB:                  %s\n", styledLocalPath(out, status.DBPath))
	if info, err := os.Stat(status.DBPath); err == nil {
		_, _ = fmt.Fprintf(out, "DB size:             %s\n", humanBytes(info.Size()))
		_, _ = fmt.Fprintf(out, "DB last modified:    %s\n", info.ModTime().Format(time.RFC3339))
	}
	_, _ = fmt.Fprintf(out, "Config path:         %s\n", styledLocalPath(out, cfgPath))
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "tlDiagram available at: %s\n", styledWebappURL(out, url))
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Run 'tld stop' to shut down the server")
}

func databaseWillBeInitialized(dataDir string) bool {
	_, err := os.Stat(localserver.DatabasePath(dataDir))
	return errors.Is(err, os.ErrNotExist)
}

func dataStatus(initialized bool) string {
	if initialized {
		return "initialized new local data"
	}
	return "using existing local data"
}

func printableMode(mode string) string {
	if mode == "" {
		return "unknown"
	}
	return mode
}

func styledLocalPath(out io.Writer, path string) string {
	return formatLocalPath(path, term.IsColorEnabled(out))
}

func styledWebappURL(out io.Writer, url string) string {
	return formatWebappURL(url, term.IsColorEnabled(out))
}

func formatLocalPath(path string, colorEnabled bool) string {
	if !colorEnabled {
		return path
	}
	return term.ColorBlue + path + term.ColorReset
}

func formatWebappURL(url string, colorEnabled bool) string {
	if !colorEnabled {
		return url
	}
	return term.ColorGreen + term.ColorUnderline + url + term.ColorReset
}

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func intPtr(v int) *int { return &v }

type readyInfo struct {
	OK        bool `json:"ok"`
	Resources struct {
		Views      int `json:"views"`
		Elements   int `json:"elements"`
		Connectors int `json:"connectors"`
	} `json:"resources"`
}

func readyResources(info *readyInfo) localserver.ResourceCounts {
	if info == nil {
		return localserver.ResourceCounts{}
	}
	return localserver.ResourceCounts{
		Views:      info.Resources.Views,
		Elements:   info.Resources.Elements,
		Connectors: info.Resources.Connectors,
	}
}

func waitReady(url string, timeout time.Duration) (*readyInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ready, err := getReady(url)
		if err == nil && ready.OK {
			return ready, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out after %s", timeout)
}

func getReady(url string) (*readyInfo, error) {
	client := &http.Client{Timeout: readyRequestTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ready status %d", resp.StatusCode)
	}
	var ready readyInfo
	if err := json.NewDecoder(resp.Body).Decode(&ready); err != nil {
		return nil, err
	}
	return &ready, nil
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
