package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	matchaCli "github.com/floatpane/matcha/cli"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/i18n"
	_ "github.com/floatpane/matcha/i18n/languages" // register locale metadata (plural rules, names)
	"github.com/floatpane/matcha/internal/cliutil"
	"github.com/floatpane/matcha/internal/daemoncmd"
	"github.com/floatpane/matcha/internal/logging"
	"github.com/floatpane/matcha/internal/loglevel"
	"github.com/floatpane/matcha/internal/oauthcmd"
	"github.com/floatpane/matcha/internal/send"
	"github.com/floatpane/matcha/internal/updater"
	"github.com/floatpane/matcha/plugin"
	"github.com/floatpane/matcha/theme"
	"github.com/floatpane/matcha/tui"
	"github.com/floatpane/matcha/view"
	"github.com/floatpane/termimage"

	"github.com/floatpane/matcha/app"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	termimage.MaybeRunWorker()

	updater.SetVersion(version)

	args, level, showLogPanel := cliutil.ParseGlobalFlags(os.Args)
	os.Args = args
	loglevel.Set(level)

	// Handle matcha: URL scheme (e.g. matcha:install:my-plugin)
	if installSlug := parseMatchaInstallURL(os.Args); installSlug != "" {
		if err := matchaCli.RunInstall([]string{installSlug}); err != nil {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	}

	runSubcommand(os.Args)

	// From here on the TUI owns the terminal: anything written to stderr
	// would be drawn over the interface, so route the standard logger to a
	// log file (and the in-app panel when --logs is given).
	logCh, logPanel := setupLogging(showLogPanel)

	if err := config.MigrateCacheFiles(); err != nil {
		log.Printf("warning: cache migration failed: %v", err)
	}

	if err := i18n.Init("en"); err != nil {
		log.Printf("Failed to initialize i18n: %v", err)
	}

	mailtoURL := parseMailto(os.Args)
	cfg := loadConfigAndSetup()

	plugins := setupPlugins(cfg)

	// Wire the plugin manager into the TUI layer so that text overrides,
	// visibility toggles, custom components, and banner overrides are
	// consulted during rendering. Must happen before HookStartup so that
	// plugins calling matcha.ui.set_banner() in their startup hook take
	// effect before the first frame.
	tui.SetUIProvider(plugins)

	initialModel := app.NewModel(cfg, mailtoURL, plugins, showLogPanel, logCh, logPanel)
	if config.IsSecureModeEnabled() {
		initialModel.SetPasswordPrompt()
	}

	setupBodyTransformer(initialModel, plugins)
	plugins.CallHook(plugin.HookStartup)

	syncNativeTheme(cfg)
	startMacOSSync(cfg)
	ensureProtocolHandler()

	// Inside tmux, image support depends on asking tmux; do it now, off the
	// UI goroutine, rather than on the first email with images.
	go view.PrimeImageDetection()

	p := tea.NewProgram(initialModel)
	if _, err := p.Run(); err != nil {
		plugins.Close()
		if errors.Is(err, tea.ErrProgramPanic) {
			// Bubble Tea may still be printing the panic report from the
			// goroutine that panicked; let it finish.
			time.Sleep(200 * time.Millisecond)
			fmt.Fprintf(os.Stderr, "\nmatcha crashed. The details are in %s; please include them in a bug report.\n", logFilePath())
		} else {
			fmt.Fprintf(os.Stderr, "Alas, there's been an error: %v\n", err)
		}
		cliutil.Exit(1)
	}

	plugins.CallHook(plugin.HookShutdown)
	plugins.Close()
	config.FlushEmailBodyCache()
	fetcher.CloseDebugFiles()
}

func runSubcommand(args []string) {
	if len(args) <= 1 {
		return
	}

	switch args[1] {
	case "-h", "--help", "help":
		printHelp()
		cliutil.Exit(0)
	case "-v", "--version", "version":
		printVersion()
		cliutil.Exit(0)
	case "update":
		if err := updater.RunUpdateCLI(); err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "daemon":
		daemoncmd.Run(args[2:])
		cliutil.Exit(0)
	case "oauth", "gmail":
		oauthcmd.Run(args[2:])
		cliutil.Exit(0)
	case "send":
		send.RunSendCLI(args[2:], cliutil.Exit)
		cliutil.Exit(0)
	case "apply":
		if err := matchaCli.RunApply(args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "apply failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "send-patch":
		if err := matchaCli.RunSendPatch(args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "send-patch failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "install":
		if err := matchaCli.RunInstall(args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "config":
		if err := matchaCli.RunConfig(args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "config failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "contacts":
		handleContactsSubcommand(args)
	case "dict":
		if err := matchaCli.RunDict(args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "dict: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "setup-mailto":
		if err := matchaCli.SetupMailto(); err != nil {
			fmt.Fprintf(os.Stderr, "setup-mailto failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "setup-protocol":
		if err := matchaCli.SetupProtocolHandler(); err != nil {
			fmt.Fprintf(os.Stderr, "setup-protocol failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "helper":
		if err := matchaCli.RunHelper(args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "helper: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "marketplace":
		_, _ = setupLogging(false)
		if cfg, err := config.LoadConfig(); err == nil {
			config.MouseEnabled = cfg.MouseEnabled
			if cfg.Theme != "" {
				theme.SetTheme(cfg.Theme)
			}
		}
		tui.RebuildStyles()
		p := tea.NewProgram(fullscreenModel{tui.NewMarketplace(true)})
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "marketplace failed: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	}
}

func handleContactsSubcommand(args []string) {
	if len(args) > 2 {
		switch args[2] {
		case "export":
			if err := matchaCli.RunContactsExport(args[3:]); err != nil {
				fmt.Fprintf(os.Stderr, "contacts export failed: %v\n", err)
				cliutil.Exit(1)
			}
			cliutil.Exit(0)
		case "sync":
			if err := matchaCli.RunContactsSync(args[3:]); err != nil {
				fmt.Fprintf(os.Stderr, "contacts sync failed: %v\n", err)
				cliutil.Exit(1)
			}
			cliutil.Exit(0)
		}
	}
}

func parseMailto(args []string) *url.URL {
	if len(args) > 1 && strings.HasPrefix(strings.ToLower(args[1]), "mailto:") {
		if u, err := url.Parse(args[1]); err == nil {
			return u
		}
	}
	return nil
}

// parseMatchaInstallURL checks whether the first argument is a
// matcha:install:<slug> URL and returns the slug.
func parseMatchaInstallURL(args []string) string {
	if len(args) <= 1 {
		return ""
	}
	arg := strings.ToLower(args[1])
	if !strings.HasPrefix(arg, "matcha:install:") {
		return ""
	}
	slug := strings.TrimSpace(args[1][len("matcha:install:"):])
	if slug == "" {
		return ""
	}
	return slug
}

func loadConfigAndSetup() *config.Config {
	if config.IsSecureModeEnabled() {
		tui.RebuildStyles()
		return nil
	}

	cfg, err := config.LoadConfig()
	if err == nil {
		loglevel.Verbosef("matcha: loaded config with %d account(s)", len(cfg.GetAccountIDs()))
		if migrateErr := config.MigrateContactsCacheUsage(cfg.GetAccountIDs()); migrateErr != nil {
			log.Printf("warning: contacts migration failed: %v", migrateErr)
		}
		if cfg.Theme != "" {
			theme.SetTheme(cfg.Theme)
		}
		lang := i18n.DetectLanguage(cfg)
		if err := i18n.GetManager().SetLanguage(lang); err != nil {
			log.Printf("Failed to set language %s: %v", lang, err)
		}
	}
	tui.RebuildStyles()
	_ = config.EnsurePGPDir()

	return cfg
}

// maxLogFileSize is the size at which matcha.log is rotated at startup.
const maxLogFileSize = 5 << 20

// setupLogging sends the standard logger to the matcha log file and, with
// --logs, to the in-app log panel. Nothing is written to stdout/stderr while
// the TUI runs, because that output would corrupt the screen.
func setupLogging(showLogPanel bool) (<-chan logging.Entry, *tui.LogPanel) {
	var writers []io.Writer
	if f := openLogFile(); f != nil {
		writers = append(writers, f)
	}

	var (
		logCh    <-chan logging.Entry
		logPanel *tui.LogPanel
	)
	if showLogPanel {
		logger := logging.NewBuffer(logging.DefaultMaxEntries)
		logCh = logger.Subscribe()
		logPanel = tui.NewLogPanel(logger)
		writers = append(writers, logger)
	}

	switch len(writers) {
	case 0:
		log.SetOutput(io.Discard)
	case 1:
		log.SetOutput(writers[0])
	default:
		log.SetOutput(io.MultiWriter(writers...))
	}
	return logCh, logPanel
}

// logFilePath returns where the matcha log is written.
func logFilePath() string {
	dir, err := config.GetCacheDir()
	if err != nil {
		return "matcha.log"
	}
	return filepath.Join(dir, "matcha.log")
}

// openLogFile opens (creating and rotating as needed) the matcha log file in
// the cache directory. It returns nil when the file cannot be opened.
func openLogFile() *os.File {
	dir, err := config.GetCacheDir()
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	path := logFilePath()
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogFileSize {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	return f
}

func setupPlugins(cfg *config.Config) *plugin.Manager {
	plugins := plugin.NewManager()
	plugins.LoadPlugins()
	if cfg != nil {
		plugins.LoadSettingValues(cfg.PluginSettings)
	}
	return plugins
}

func setupBodyTransformer(initialModel *app.Model, plugins *plugin.Manager) {
	tui.BodyTransformer = func(body string, email fetcher.Email) string {
		folder := "INBOX"
		if initialModel.FolderInbox() != nil {
			folder = initialModel.FolderInbox().GetCurrentFolder()
		}
		t := plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, folder)
		return plugins.CallBodyRenderHook(t, body, email.Body)
	}
}

func startMacOSSync(cfg *config.Config) {
	if runtime.GOOS != "darwin" {
		return
	}

	disableNotifications := false
	if cfg != nil {
		disableNotifications = cfg.DisableNotifications
	}
	if disableNotifications {
		return
	}

	var accountIDs []string
	if cfg != nil {
		accountIDs = cfg.GetAccountIDs()
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic in macOS sync goroutine: %v", r)
			}
		}()
		// Reuse the loaded config: SyncMacOSContacts reloads it, re-running
		// pass_cmd and keyring lookups while the TUI owns the terminal.
		_ = config.SyncMacOSContactsForAccounts(accountIDs)
	}()
}

// syncNativeTheme applies the macOS appearance before the program starts:
// the renderer reads theme.ActiveTheme without synchronization, so it must
// not be written from a background goroutine while the TUI runs.
func syncNativeTheme(cfg *config.Config) {
	if runtime.GOOS != "darwin" || (cfg != nil && cfg.DisableNotifications) {
		return
	}
	if t, err := theme.MacOSNativeTheme(); err == nil {
		theme.SetNative(t)
		tui.RebuildStyles()
	}
}

// ensureProtocolHandler registers the matcha: URL scheme handler once.
// A sentinel file (~/.config/matcha/.protocol_registered) prevents
// re-running on every launch. Failures are logged but never fatal.
func ensureProtocolHandler() {
	dir, err := config.GetConfigDir()
	if err != nil {
		return
	}
	sentinel := filepath.Join(dir, ".protocol_registered")
	if _, err := os.Stat(sentinel); err == nil {
		return // already registered
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic in protocol handler setup: %v", r)
			}
		}()
		// The CLI helper prints progress; the TUI owns the screen now.
		if err := matchaCli.SetupProtocolHandlerTo(io.Discard); err != nil {
			// Remember the attempt anyway: retrying rebuilds the app bundle on
			// every launch. `matcha setup-protocol` can be run by hand.
			log.Printf("warning: could not register matcha: protocol: %v", err)
		}
		_ = os.WriteFile(sentinel, []byte("1"), 0644)
	}()
}

func printHelp() {
	fmt.Print(`matcha - a terminal email client

Usage:
  matcha [flags]            Launch the TUI (default)
  matcha <command> [args]

Commands:
  send             Send an email from the command line
  apply            Apply a patch from a git-format-patch email
  send-patch       Send a git-format-patch email
  contacts         Manage contacts (export, sync)
  config           View or edit configuration
  dict             Manage the spellcheck dictionary
  install          Install a plugin
  marketplace      Browse the plugin marketplace
  daemon           Run the background sync daemon
  oauth, gmail     Manage OAuth / Gmail authentication
  setup-mailto     Register matcha as the mailto: handler
  setup-protocol   Register the matcha: URL scheme handler
  update           Update matcha to the latest release
  version          Print version information
  help             Show this help

Flags:
  --debug          Enable debug logging
  --verbose, -V    Enable verbose logging
  --logs           Show the in-app log panel
  -h, --help       Show this help
  -v, --version    Print version information

Run 'matcha <command> --help' for more information on a command.
`)
}

func printVersion() {
	fmt.Printf("matcha version %s", version)
	if commit != "" {
		fmt.Printf(" (%s)", commit)
	}
	if date != "" {
		fmt.Printf(" built on %s", date)
	}
	fmt.Println()
}

// fullscreenModel runs a standalone TUI screen in the alternate screen with
// the configured mouse mode, like the main application does.
type fullscreenModel struct {
	tea.Model
}

func (m fullscreenModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.Model.Update(msg)
	return fullscreenModel{next}, cmd
}

func (m fullscreenModel) View() tea.View {
	v := m.Model.View()
	v.AltScreen = true
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
