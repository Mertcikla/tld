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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

const (
	backgroundReadyTimeout = 30 * time.Second
	readyRequestTimeout    = 10 * time.Second
	skipStartupUpdateEnv   = "TLD_SKIP_STARTUP_UPDATE"
)

func defaultServeRunE(cmd *cobra.Command, args []string) error {
	_ = workspace.EnsureGlobalConfig()

	foreground, _ := cmd.Flags().GetBool("foreground")
	openBrowser, _ := cmd.Flags().GetBool("open")
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetString("port")
	dataDirFlag, _ := cmd.Flags().GetString("data-dir")

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return err
	}
	dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
	if err != nil {
		return err
	}
	workspaceDir, err := resolveCommandWorkspaceDir(cmd)
	if err != nil {
		return err
	}

	if foreground {
		return runForeground(cmd, host, port, dataDir, workspaceDir, openBrowser)
	}
	return runBackground(cmd, host, port, dataDir, workspaceDir, openBrowser)
}

func runForeground(cmd *cobra.Command, host, port, dataDir, workspaceDir string, openBrowser bool) error {
	started := time.Now()
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return err
	}
	opts := resolveServeOptions(cfg, host, port)
	opts.Config = cfg
	opts.WorkspaceDir = workspaceDir

	app, err := localserver.Bootstrap(dataDir, opts)
	if err != nil {
		return err
	}
	if err := localserver.RegisterProcess(localserver.ProcessRecord{
		Kind:    localserver.ProcessKindServer,
		PID:     os.Getpid(),
		DataDir: dataDir,
		Addr:    app.Addr,
	}); err != nil {
		return err
	}
	defer func() { _ = localserver.RemoveProcess(os.Getpid()) }()

	PrintLogo(cmd.OutOrStdout())
	if os.Getenv(skipStartupUpdateEnv) != "1" {
		go reportStartupUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cfg)
	}
	url := localserver.DisplayURL(opts, app.Addr)
	printServeInfo(cmd.OutOrStdout(), url, serveStatus{
		Mode:            "foreground",
		InitializedData: app.InitializedData,
		Resources:       app.Resources,
		BindAddr:        app.Addr,
		Startup:         time.Since(started),
		DBPath:          app.DBPath,
		DBDriver:        app.DBDriver,
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

func runBackground(cmd *cobra.Command, host, port, dataDir, workspaceDir string, openBrowser bool) error {
	started := time.Now()
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return err
	}
	opts := resolveServeOptions(cfg, host, port)
	addr := localserver.ResolveAddr(opts)
	readyURL := "http://" + addr
	url := localserver.DisplayURL(opts, addr)
	initializedData := databaseWillBeInitialized(cfg, dataDir)

	if existing, ok := findRunningServerProcess(dataDir, addr); ok {
		PrintLogo(cmd.OutOrStdout())
		if existing.Addr != "" {
			addr = existing.Addr
			readyURL = "http://" + addr
			url = localserver.DisplayURL(opts, addr)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Server already running (pid %d)\n", existing.PID)
		ready, _ := getReady(readyURL + "/api/ready")
		printServeInfo(cmd.OutOrStdout(), url, serveStatus{
			Mode:            "background",
			InitializedData: initializedData,
			Resources:       readyResources(ready),
			PID:             new(existing.PID),
			BindAddr:        addr,
			Startup:         0,
			DBPath:          localserver.DatabasePath(dataDir),
			DBDriver:        cfg.Database.Driver,
		})
		if openBrowser {
			_ = cmdutil.OpenBrowser(url)
		}
		reportStartupUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cfg)
		return nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	fwdArgs := []string{}
	if workspaceDir != "" {
		fwdArgs = append(fwdArgs, "--workspace", workspaceDir)
	}
	fwdArgs = append(fwdArgs, "serve", "--foreground")
	if opts.Host != "" {
		fwdArgs = append(fwdArgs, "--host", opts.Host)
	}
	if opts.Port != "" {
		fwdArgs = append(fwdArgs, "--port", opts.Port)
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
	child.Env = append(os.Environ(), skipStartupUpdateEnv+"=1")
	child.SysProcAttr = getSysProcAttr()

	if err := child.Start(); err != nil {
		return fmt.Errorf("start server process: %w", err)
	}

	ready, err := waitReady(readyURL+"/api/ready", backgroundReadyTimeout)
	if err != nil {
		_ = child.Process.Kill()
		_ = localserver.RemoveProcess(child.Process.Pid)
		return fmt.Errorf("server did not become ready: %w\nCheck logs: %s", err, localserver.LogPath(dataDir))
	}

	if !localserver.IsRunning(child.Process.Pid) {
		_ = localserver.RemoveProcess(child.Process.Pid)
		return fmt.Errorf("server process exited immediately; check logs: %s", localserver.LogPath(dataDir))
	}

	PrintLogo(cmd.OutOrStdout())
	printServeInfo(cmd.OutOrStdout(), url, serveStatus{
		Mode:            "background",
		InitializedData: initializedData,
		Resources:       readyResources(ready),
		PID:             new(child.Process.Pid),
		BindAddr:        addr,
		Startup:         time.Since(started),
		DBPath:          localserver.DatabasePath(dataDir),
		DBDriver:        cfg.Database.Driver,
	})

	if openBrowser {
		_ = cmdutil.OpenBrowser(url)
	}
	reportStartupUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cfg)
	return nil
}

func reportStartupUpdate(ctx context.Context, out, progress io.Writer, cfg *workspace.Config) {
	updateStatus, updateNote := StartupUpdateStatus(ctx, cfg, progress)
	if updateStatus != nil && updateStatus.UpdateAvailable {
		term.Hint(out, fmt.Sprintf("Update available: %s -> %s", updateStatus.Current, updateStatus.Latest))
	}
	if updateNote != "" {
		term.Hint(out, updateNote)
	}
}

func findRunningServerProcess(dataDir, addr string) (localserver.ProcessRecord, bool) {
	reg, err := localserver.PruneProcessRegistry()
	if err != nil {
		return localserver.ProcessRecord{}, false
	}
	for _, proc := range reg.Processes {
		if proc.Addr == addr {
			return proc, true
		}
		if proc.Kind == localserver.ProcessKindServer && proc.DataDir == dataDir {
			return proc, true
		}
	}
	return localserver.ProcessRecord{}, false
}

type serveStatus struct {
	Mode            string
	PID             *int
	BindAddr        string
	InitializedData bool
	Resources       localserver.ResourceCounts
	Startup         time.Duration
	DBPath          string
	DBDriver        string
}

func printServeInfo(out io.Writer, url string, status serveStatus) {
	cfgPath, _ := workspace.ConfigPath()
	term.Label(out, 20, "Mode", printableMode(status.Mode))
	if status.PID != nil {
		term.Label(out, 20, "PID", fmt.Sprintf("%d", *status.PID))
	}
	term.Label(out, 20, "Server status", dataStatus(status.InitializedData, status.DBDriver))
	term.Label(out, 20, "Bind address", status.BindAddr)
	if !status.InitializedData {
		term.Label(out, 20, "Resource counts", fmt.Sprintf("%d views, %d elements, %d connectors", status.Resources.Views, status.Resources.Elements, status.Resources.Connectors))
	}
	if status.Startup > 0 {
		term.Label(out, 20, "Ready in", status.Startup.Round(time.Millisecond).String())
	}
	term.Label(out, 20, "DB", databaseLabel(out, status))
	if status.DBPath != "" && normalizedDBDriver(status.DBDriver) == "sqlite" {
		if info, err := os.Stat(status.DBPath); err == nil {
			term.Label(out, 20, "DB size", humanBytes(info.Size()))
			term.Label(out, 20, "DB last modified", info.ModTime().Format(time.RFC3339))
		}
	}
	term.Label(out, 20, "Config path", term.Path(out, cfgPath))
	term.Separator(out)
	_, _ = fmt.Fprintf(out, "  tlDiagram available at: %s\n", term.URL(out, url))
	term.Separator(out)
	term.Hint(out, "Run 'tld stop' to shut down the server")
}

func databaseWillBeInitialized(cfg *workspace.Config, dataDir string) bool {
	driver := ""
	if cfg != nil {
		driver = cfg.Database.Driver
	}
	if normalizedDBDriver(driver) != "sqlite" {
		return false
	}
	_, err := os.Stat(localserver.DatabasePath(dataDir))
	return errors.Is(err, os.ErrNotExist)
}

func dataStatus(initialized bool, driver string) string {
	if normalizedDBDriver(driver) == "postgres" {
		return "using postgres database"
	}
	if initialized {
		return "initialized new local data"
	}
	return "using existing local data"
}

func databaseLabel(out io.Writer, status serveStatus) string {
	switch normalizedDBDriver(status.DBDriver) {
	case "postgres":
		return "postgres"
	default:
		if status.DBPath == "" {
			return "sqlite"
		}
		return term.Path(out, status.DBPath)
	}
}

func normalizedDBDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql":
		return "postgres"
	default:
		return "sqlite"
	}
}

func printableMode(mode string) string {
	if mode == "" {
		return "unknown"
	}
	return mode
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

func resolveServeOptions(cfg *workspace.Config, flagHost, flagPort string) localserver.ServeOptions {
	serve := workspace.ResolveServeOptions(cfg, flagHost, flagPort)
	return localserver.ServeOptions{
		Host:           serve.Host,
		Port:           serve.Port,
		PublicURL:      serve.PublicURL,
		AllowedOrigins: serve.AllowedOrigins,
	}
}

func resolveCommandWorkspaceDir(cmd *cobra.Command) (string, error) {
	root := cmd.Root()
	if root == nil {
		return "", nil
	}
	flag := root.PersistentFlags().Lookup("workspace")
	if flag == nil {
		return "", nil
	}
	value := strings.TrimSpace(flag.Value.String())
	if value == "" {
		return "", nil
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve workspace directory: %w", err)
	}
	return abs, nil
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
