package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	matchaCli "github.com/floatpane/matcha/cli"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/i18n"
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

	runSubcommand(os.Args)

	if err := config.MigrateCacheFiles(); err != nil {
		log.Printf("warning: cache migration failed: %v", err)
	}

	if err := i18n.Init("en"); err != nil {
		log.Printf("Failed to initialize i18n: %v", err)
	}

	mailtoURL := parseMailto(os.Args)
	cfg := loadConfigAndSetup()

	_, logCh, logPanel := setupLogging(showLogPanel)
	plugins := setupPlugins(cfg)

	initialModel := app.NewModel(cfg, mailtoURL, plugins, showLogPanel, logCh, logPanel)
	if config.IsSecureModeEnabled() {
		initialModel.SetPasswordPrompt()
	}

	setupBodyTransformer(initialModel, plugins)
	plugins.CallHook(plugin.HookStartup)

	startMacOSSync(cfg)

	p := tea.NewProgram(initialModel)
	if _, err := p.Run(); err != nil {
		plugins.Close()
		fmt.Printf("Alas, there's been an error: %v", err)
		cliutil.Exit(1)
	}

	plugins.CallHook(plugin.HookShutdown)
	plugins.Close()
	fetcher.CloseDebugFiles()
}

func runSubcommand(args []string) {
	if len(args) <= 1 {
		return
	}

	switch args[1] {
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
	case "helper":
		if err := matchaCli.RunHelper(args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "helper: %v\n", err)
			cliutil.Exit(1)
		}
		cliutil.Exit(0)
	case "marketplace":
		mp := tui.NewMarketplace(true)
		p := tea.NewProgram(mp)
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

func setupLogging(showLogPanel bool) (*logging.Buffer, <-chan logging.Entry, *tui.LogPanel) {
	if !showLogPanel {
		return nil, nil, nil
	}

	logger := logging.NewBuffer(logging.DefaultMaxEntries)
	log.SetOutput(logger)
	logCh := logger.Subscribe()
	logPanel := tui.NewLogPanel(logger)
	return logger, logCh, logPanel
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

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic in macOS sync goroutine: %v", r)
			}
		}()
		_ = config.SyncMacOSContacts()
		_ = theme.SyncWithMacOS()
	}()
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
