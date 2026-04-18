package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

const keyringServiceName = "matcha-email-client"

// Date format presets use human-readable tokens. Supported tokens:
//
//	YYYY (4-digit year), YY (2-digit year)
//	MM   (month, or minutes when following an hour token + colon)
//	mm   (minutes, explicit)
//	DD   (day)
//	HH   (24-hour), hh (12-hour, zero-padded)
//	SS, ss (seconds)
//	AM, PM (meridiem marker)
const (
	DateFormatISO = "YYYY-MM-DD HH:MM"
	DateFormatUS  = "MM/DD/YYYY hh:MM AM"
	DateFormatEU  = "DD/MM/YYYY HH:MM"
)

// Account stores the configuration for a single email account.
type Account struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Password        string `json:"-"`                // "-" prevents the password from being saved to config.json
	ServiceProvider string `json:"service_provider"` // "gmail", "outlook", "icloud", or "custom"
	// FetchEmail is the single email address for which messages should be fetched.
	// If empty, it will default to `Email` when accounts are added.
	FetchEmail string `json:"fetch_email,omitempty"`
	// SendAsEmail controls the visible From header on outgoing mail.
	// If empty, it defaults to FetchEmail, then Email.
	SendAsEmail string `json:"send_as_email,omitempty"`

	// Custom server settings (used when ServiceProvider is "custom")
	IMAPServer string `json:"imap_server,omitempty"`
	IMAPPort   int    `json:"imap_port,omitempty"`
	SMTPServer string `json:"smtp_server,omitempty"`
	SMTPPort   int    `json:"smtp_port,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"`

	// S/MIME settings
	SMIMECert          string `json:"smime_cert,omitempty"`            // Path to the public certificate PEM
	SMIMEKey           string `json:"smime_key,omitempty"`             // Path to the private key PEM
	SMIMESignByDefault bool   `json:"smime_sign_by_default,omitempty"` // Whether to enable S/MIME signing by default

	// PGP settings
	PGPPublicKey     string `json:"pgp_public_key,omitempty"`      // Path to public key (.asc or .gpg)
	PGPPrivateKey    string `json:"pgp_private_key,omitempty"`     // Path to private key (.asc or .gpg)
	PGPKeySource     string `json:"pgp_key_source,omitempty"`      // "file" (default) or "yubikey" for hardware key
	PGPPIN           string `json:"-"`                             // YubiKey PIN (stored in keyring, not JSON)
	PGPSignByDefault bool   `json:"pgp_sign_by_default,omitempty"` // Auto-sign outgoing emails

	// OAuth2 settings
	AuthMethod string `json:"auth_method,omitempty"` // "password" (default) or "oauth2"

	// Multi-protocol settings
	Protocol     string `json:"protocol,omitempty"`      // "imap" (default), "jmap", or "pop3"
	JMAPEndpoint string `json:"jmap_endpoint,omitempty"` // JMAP session URL (for protocol=jmap)
	POP3Server   string `json:"pop3_server,omitempty"`   // POP3 server hostname (for protocol=pop3)
	POP3Port     int    `json:"pop3_port,omitempty"`     // POP3 server port (for protocol=pop3)
}

// MailingList represents a named group of email addresses.
type MailingList struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
}

// Config stores the user's email configuration with multiple accounts.
type Config struct {
	Accounts             []Account     `json:"accounts"`
	DisableImages        bool          `json:"disable_images,omitempty"`
	HideTips             bool          `json:"hide_tips,omitempty"`
	DisableNotifications bool          `json:"disable_notifications,omitempty"`
	Theme                string        `json:"theme,omitempty"`
	MailingLists         []MailingList `json:"mailing_lists,omitempty"`
	DateFormat           string        `json:"date_format,omitempty"`
}

// GetDateFormat returns the Go time reference layout translated from the
// user's configured human-readable format. Defaults to EU when unset.
func (c *Config) GetDateFormat() string {
	f := c.DateFormat
	if f == "" {
		f = DateFormatEU
	}
	return translateDateFormat(f)
}

// translateDateFormat converts a human-readable format string (e.g.
// "DD/MM/YYYY HH:MM") into a Go reference-time layout usable by
// time.Format. MM is disambiguated by context: when it directly follows
// an hour token plus ":", it maps to minutes; otherwise to month.
func translateDateFormat(f string) string {
	var b strings.Builder
	i := 0
	for i < len(f) {
		rest := f[i:]
		switch {
		case strings.HasPrefix(rest, "YYYY"):
			b.WriteString("2006")
			i += 4
		case strings.HasPrefix(rest, "YY"):
			b.WriteString("06")
			i += 2
		case strings.HasPrefix(rest, "DD"):
			b.WriteString("02")
			i += 2
		case strings.HasPrefix(rest, "HH"):
			b.WriteString("15")
			i += 2
		case strings.HasPrefix(rest, "hh"):
			b.WriteString("03")
			i += 2
		case strings.HasPrefix(rest, "mm"):
			b.WriteString("04")
			i += 2
		case strings.HasPrefix(rest, "SS"), strings.HasPrefix(rest, "ss"):
			b.WriteString("05")
			i += 2
		case strings.HasPrefix(rest, "MM"):
			cur := b.String()
			if strings.HasSuffix(cur, "15:") || strings.HasSuffix(cur, "03:") {
				b.WriteString("04")
			} else {
				b.WriteString("01")
			}
			i += 2
		case strings.HasPrefix(rest, "AM"), strings.HasPrefix(rest, "PM"):
			b.WriteString("PM")
			i += 2
		default:
			b.WriteByte(f[i])
			i++
		}
	}
	return b.String()
}

// GetIMAPServer returns the IMAP server address for the account.
func (a *Account) GetIMAPServer() string {
	switch a.ServiceProvider {
	case "gmail":
		return "imap.gmail.com"
	case "outlook":
		return "outlook.office365.com"
	case "icloud":
		return "imap.mail.me.com"
	case "custom":
		return a.IMAPServer
	default:
		return ""
	}
}

// GetIMAPPort returns the IMAP port for the account.
func (a *Account) GetIMAPPort() int {
	switch a.ServiceProvider {
	case "gmail", "outlook", "icloud":
		return 993
	case "custom":
		if a.IMAPPort != 0 {
			return a.IMAPPort
		}
		return 993 // Default IMAP SSL port
	default:
		return 993
	}
}

// GetSMTPServer returns the SMTP server address for the account.
func (a *Account) GetSMTPServer() string {
	switch a.ServiceProvider {
	case "gmail":
		return "smtp.gmail.com"
	case "outlook":
		return "smtp.office365.com"
	case "icloud":
		return "smtp.mail.me.com"
	case "custom":
		return a.SMTPServer
	default:
		return ""
	}
}

// GetSMTPPort returns the SMTP port for the account.
func (a *Account) GetSMTPPort() int {
	switch a.ServiceProvider {
	case "gmail", "outlook", "icloud":
		return 587
	case "custom":
		if a.SMTPPort != 0 {
			return a.SMTPPort
		}
		return 587 // Default SMTP TLS port
	default:
		return 587
	}
}

// GetFetchEmail returns the configured fetch identity, falling back to Email.
func (a *Account) GetFetchEmail() string {
	if a.FetchEmail != "" {
		return a.FetchEmail
	}
	return a.Email
}

// GetSendAsEmail returns the visible sender address for outgoing mail.
func (a *Account) GetSendAsEmail() string {
	if a.SendAsEmail != "" {
		return a.SendAsEmail
	}
	return a.GetFetchEmail()
}

// FormatFromHeader returns the display-ready From header value.
func (a *Account) FormatFromHeader() string {
	sendAs := a.GetSendAsEmail()
	if a.Name != "" && sendAs != "" {
		return fmt.Sprintf("%s <%s>", a.Name, sendAs)
	}
	return sendAs
}

// GetPOP3Server returns the POP3 server address for the account.
func (a *Account) GetPOP3Server() string {
	if a.POP3Server != "" {
		return a.POP3Server
	}
	return ""
}

// GetPOP3Port returns the POP3 port for the account.
func (a *Account) GetPOP3Port() int {
	if a.POP3Port != 0 {
		return a.POP3Port
	}
	return 995 // Default POP3 SSL port
}

// GetConfigDir returns the path to the configuration directory (exported).
func GetConfigDir() (string, error) {
	return configDir()
}

// configDir returns the path to the configuration directory (internal).
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "matcha"), nil
}

// GetCacheDir returns the path to the cache directory (exported).
func GetCacheDir() (string, error) {
	return cacheDir()
}

// cacheDir returns the path to the cache directory (internal).
func cacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "matcha"), nil
}

// MigrateCacheFiles moves cache files from ~/.config/matcha/ to ~/.cache/matcha/ if needed.
// This is a one-time migration for existing installations.
func MigrateCacheFiles() error {
	src, err := configDir()
	if err != nil {
		return err
	}
	dst, err := cacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}

	// Files to migrate
	files := []string{"email_cache.json", "contacts.json", "drafts.json", "folder_cache.json"}
	for _, f := range files {
		oldPath := filepath.Join(src, f)
		newPath := filepath.Join(dst, f)
		if _, err := os.Stat(oldPath); err == nil {
			// Only migrate if destination doesn't already exist
			if _, err := os.Stat(newPath); err != nil {
				if err := os.Rename(oldPath, newPath); err != nil {
					return err
				}
			}
		}
	}

	// Migrate folder_emails directory
	oldDir := filepath.Join(src, "folder_emails")
	newDir := filepath.Join(dst, "folder_emails")
	if info, err := os.Stat(oldDir); err == nil && info.IsDir() {
		if _, err := os.Stat(newDir); err != nil {
			if err := os.Rename(oldDir, newDir); err != nil {
				return err
			}
		}
	}

	return nil
}

// configFile returns the full path to the configuration file.
func configFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// secureDiskAccount includes the Password field in JSON when secure mode is active.
type secureDiskAccount struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Email              string `json:"email"`
	Password           string `json:"password,omitempty"`
	ServiceProvider    string `json:"service_provider"`
	FetchEmail         string `json:"fetch_email,omitempty"`
	SendAsEmail        string `json:"send_as_email,omitempty"`
	IMAPServer         string `json:"imap_server,omitempty"`
	IMAPPort           int    `json:"imap_port,omitempty"`
	SMTPServer         string `json:"smtp_server,omitempty"`
	SMTPPort           int    `json:"smtp_port,omitempty"`
	Insecure           bool   `json:"insecure,omitempty"`
	SMIMECert          string `json:"smime_cert,omitempty"`
	SMIMEKey           string `json:"smime_key,omitempty"`
	SMIMESignByDefault bool   `json:"smime_sign_by_default,omitempty"`
	PGPPublicKey       string `json:"pgp_public_key,omitempty"`
	PGPPrivateKey      string `json:"pgp_private_key,omitempty"`
	PGPKeySource       string `json:"pgp_key_source,omitempty"`
	PGPPIN             string `json:"pgp_pin,omitempty"`
	PGPSignByDefault   bool   `json:"pgp_sign_by_default,omitempty"`
	AuthMethod         string `json:"auth_method,omitempty"`
	Protocol           string `json:"protocol,omitempty"`
	JMAPEndpoint       string `json:"jmap_endpoint,omitempty"`
	POP3Server         string `json:"pop3_server,omitempty"`
	POP3Port           int    `json:"pop3_port,omitempty"`
}

type secureDiskConfig struct {
	Accounts             []secureDiskAccount `json:"accounts"`
	DisableImages        bool                `json:"disable_images,omitempty"`
	HideTips             bool                `json:"hide_tips,omitempty"`
	DisableNotifications bool                `json:"disable_notifications,omitempty"`
	Theme                string              `json:"theme,omitempty"`
	MailingLists         []MailingList       `json:"mailing_lists,omitempty"`
	DateFormat           string              `json:"date_format,omitempty"`
}

// SaveConfig saves the given configuration to the config file and passwords to the keyring.
func SaveConfig(config *Config) error {
	secureMode := GetSessionKey() != nil

	if !secureMode {
		// Save passwords and PGP PINs to the OS keyring before writing the JSON file.
		// A silent keyring failure here would lose the credential on restart without
		// any hint to the user. Log the error as a warning so the misconfiguration
		// (no keyring backend, locked keyring, etc.) is at least visible. See #616.
		for _, acc := range config.Accounts {
			if acc.Password != "" {
				if err := keyring.Set(keyringServiceName, acc.Email, acc.Password); err != nil {
					log.Printf("matcha: failed to store password for %s in keyring: %v", acc.Email, err)
				}
			}
			if acc.PGPPIN != "" && acc.PGPKeySource == "yubikey" {
				if err := keyring.Set(keyringServiceName, acc.Email+":pgp-pin", acc.PGPPIN); err != nil {
					log.Printf("matcha: failed to store PGP PIN for %s in keyring: %v", acc.Email, err)
				}
			}
		}
	}

	path, err := configFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	var data []byte
	if secureMode {
		// In secure mode, include passwords in the JSON (they'll be encrypted on disk)
		sdc := secureDiskConfig{
			DisableImages:        config.DisableImages,
			HideTips:             config.HideTips,
			DisableNotifications: config.DisableNotifications,
			Theme:                config.Theme,
			MailingLists:         config.MailingLists,
			DateFormat:           config.DateFormat,
		}
		for _, acc := range config.Accounts {
			sdc.Accounts = append(sdc.Accounts, secureDiskAccount{
				ID:                 acc.ID,
				Name:               acc.Name,
				Email:              acc.Email,
				Password:           acc.Password,
				ServiceProvider:    acc.ServiceProvider,
				FetchEmail:         acc.FetchEmail,
				SendAsEmail:        acc.SendAsEmail,
				IMAPServer:         acc.IMAPServer,
				IMAPPort:           acc.IMAPPort,
				SMTPServer:         acc.SMTPServer,
				SMTPPort:           acc.SMTPPort,
				Insecure:           acc.Insecure,
				SMIMECert:          acc.SMIMECert,
				SMIMEKey:           acc.SMIMEKey,
				SMIMESignByDefault: acc.SMIMESignByDefault,
				PGPPublicKey:       acc.PGPPublicKey,
				PGPPrivateKey:      acc.PGPPrivateKey,
				PGPKeySource:       acc.PGPKeySource,
				PGPPIN:             acc.PGPPIN,
				PGPSignByDefault:   acc.PGPSignByDefault,
				AuthMethod:         acc.AuthMethod,
				Protocol:           acc.Protocol,
				JMAPEndpoint:       acc.JMAPEndpoint,
				POP3Server:         acc.POP3Server,
				POP3Port:           acc.POP3Port,
			})
		}
		data, err = json.MarshalIndent(sdc, "", "  ")
	} else {
		data, err = json.MarshalIndent(config, "", "  ")
	}
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// LoadConfig loads the configuration from the config file and passwords from the keyring.
// It automatically migrates plain-text passwords to the OS keyring if they exist.
func LoadConfig() (*Config, error) {
	path, err := configFile()
	if err != nil {
		return nil, err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		return nil, err
	}

	secureMode := GetSessionKey() != nil

	var config Config
	var needsMigration bool

	type rawAccount struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		Email              string `json:"email"`
		Password           string `json:"password,omitempty"`
		ServiceProvider    string `json:"service_provider"`
		FetchEmail         string `json:"fetch_email,omitempty"`
		SendAsEmail        string `json:"send_as_email,omitempty"`
		IMAPServer         string `json:"imap_server,omitempty"`
		IMAPPort           int    `json:"imap_port,omitempty"`
		SMTPServer         string `json:"smtp_server,omitempty"`
		SMTPPort           int    `json:"smtp_port,omitempty"`
		Insecure           bool   `json:"insecure,omitempty"`
		SMIMECert          string `json:"smime_cert,omitempty"`
		SMIMEKey           string `json:"smime_key,omitempty"`
		SMIMESignByDefault bool   `json:"smime_sign_by_default,omitempty"`
		PGPPublicKey       string `json:"pgp_public_key,omitempty"`
		PGPPrivateKey      string `json:"pgp_private_key,omitempty"`
		PGPKeySource       string `json:"pgp_key_source,omitempty"`
		PGPPIN             string `json:"pgp_pin,omitempty"`
		PGPSignByDefault   bool   `json:"pgp_sign_by_default,omitempty"`
		AuthMethod         string `json:"auth_method,omitempty"`
		Protocol           string `json:"protocol,omitempty"`
		JMAPEndpoint       string `json:"jmap_endpoint,omitempty"`
		POP3Server         string `json:"pop3_server,omitempty"`
		POP3Port           int    `json:"pop3_port,omitempty"`
	}
	type diskConfig struct {
		Accounts             []rawAccount  `json:"accounts"`
		DisableImages        bool          `json:"disable_images,omitempty"`
		HideTips             bool          `json:"hide_tips,omitempty"`
		DisableNotifications bool          `json:"disable_notifications,omitempty"`
		Theme                string        `json:"theme,omitempty"`
		MailingLists         []MailingList `json:"mailing_lists,omitempty"`
		DateFormat           string        `json:"date_format,omitempty"`
	}

	var raw diskConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		var legacyConfig legacyConfigFormat
		if legacyErr := json.Unmarshal(data, &legacyConfig); legacyErr == nil && legacyConfig.Email != "" {
			config = Config{
				Accounts: []Account{
					{
						ID:              uuid.New().String(),
						Name:            legacyConfig.Name,
						Email:           legacyConfig.Email,
						Password:        legacyConfig.Password,
						ServiceProvider: legacyConfig.ServiceProvider,
						FetchEmail:      legacyConfig.Email,
					},
				},
			}
			// SaveConfig automatically pushes the password to the keyring and strips it from JSON
			if saveErr := SaveConfig(&config); saveErr != nil {
				return nil, saveErr
			}
			return &config, nil
		}
		return nil, err
	}

	config.DisableImages = raw.DisableImages
	config.HideTips = raw.HideTips
	config.DisableNotifications = raw.DisableNotifications
	config.Theme = raw.Theme
	config.MailingLists = raw.MailingLists
	config.DateFormat = raw.DateFormat
	for _, rawAcc := range raw.Accounts {
		acc := Account{
			ID:                 rawAcc.ID,
			Name:               rawAcc.Name,
			Email:              rawAcc.Email,
			ServiceProvider:    rawAcc.ServiceProvider,
			FetchEmail:         rawAcc.FetchEmail,
			SendAsEmail:        rawAcc.SendAsEmail,
			IMAPServer:         rawAcc.IMAPServer,
			IMAPPort:           rawAcc.IMAPPort,
			SMTPServer:         rawAcc.SMTPServer,
			SMTPPort:           rawAcc.SMTPPort,
			Insecure:           rawAcc.Insecure,
			SMIMECert:          rawAcc.SMIMECert,
			SMIMEKey:           rawAcc.SMIMEKey,
			SMIMESignByDefault: rawAcc.SMIMESignByDefault,
			PGPPublicKey:       rawAcc.PGPPublicKey,
			PGPPrivateKey:      rawAcc.PGPPrivateKey,
			PGPKeySource:       rawAcc.PGPKeySource,
			PGPSignByDefault:   rawAcc.PGPSignByDefault,
			AuthMethod:         rawAcc.AuthMethod,
			Protocol:           rawAcc.Protocol,
			JMAPEndpoint:       rawAcc.JMAPEndpoint,
			POP3Server:         rawAcc.POP3Server,
			POP3Port:           rawAcc.POP3Port,
		}

		// Validate PGPKeySource
		if acc.PGPKeySource != "" && acc.PGPKeySource != "file" && acc.PGPKeySource != "yubikey" {
			return nil, fmt.Errorf("account %q: invalid pgp_key_source %q (must be \"file\" or \"yubikey\")", acc.Name, acc.PGPKeySource)
		}

		if secureMode {
			// In secure mode, passwords and PINs are stored in the encrypted config JSON
			acc.Password = rawAcc.Password
			acc.PGPPIN = rawAcc.PGPPIN
		} else if rawAcc.Password != "" {
			// Found a plain-text password! Move it to the OS Keyring.
			if err := keyring.Set(keyringServiceName, rawAcc.Email, rawAcc.Password); err != nil {
				log.Printf("matcha: failed to migrate password for %s into keyring: %v", rawAcc.Email, err)
			}
			acc.Password = rawAcc.Password
			needsMigration = true
		} else {
			// No plaintext password in JSON, fetch from Keyring as normal.
			if pwd, err := keyring.Get(keyringServiceName, acc.Email); err == nil {
				acc.Password = pwd
			}
		}

		if !secureMode {
			// Load YubiKey PIN from keyring if using YubiKey
			if acc.PGPKeySource == "yubikey" {
				if pin, err := keyring.Get(keyringServiceName, acc.Email+":pgp-pin"); err == nil {
					acc.PGPPIN = pin
				}
			}
		}

		config.Accounts = append(config.Accounts, acc)
	}

	if needsMigration {
		if saveErr := SaveConfig(&config); saveErr != nil {
			return nil, saveErr
		}
	}

	return &config, nil
}

// legacyConfigFormat represents the old single-account configuration format.
type legacyConfigFormat struct {
	ServiceProvider string `json:"service_provider"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	Name            string `json:"name"`
}

// AddAccount adds a new account to the configuration.
func (c *Config) AddAccount(account Account) {
	if account.ID == "" {
		account.ID = uuid.New().String()
	}
	// Ensure FetchEmail defaults to the login Email if not explicitly set.
	if account.FetchEmail == "" && account.Email != "" {
		account.FetchEmail = account.Email
	}
	c.Accounts = append(c.Accounts, account)
}

// RemoveAccount removes an account by its ID and deletes its password from the keyring.
func (c *Config) RemoveAccount(id string) bool {
	for i, acc := range c.Accounts {
		if acc.ID == id {
			// Delete password from OS Keyring when account is removed. A
			// missing entry is expected and not worth logging (keyring.Get is
			// what we rely on elsewhere to detect that), but any other error
			// means we failed to clean up a still-reachable secret.
			if err := keyring.Delete(keyringServiceName, acc.Email); err != nil && err != keyring.ErrNotFound {
				log.Printf("matcha: failed to delete password for %s from keyring: %v", acc.Email, err)
			}
			// Delete PGP PIN from OS Keyring if present
			if err := keyring.Delete(keyringServiceName, acc.Email+":pgp-pin"); err != nil && err != keyring.ErrNotFound {
				log.Printf("matcha: failed to delete PGP PIN for %s from keyring: %v", acc.Email, err)
			}

			c.Accounts = append(c.Accounts[:i], c.Accounts[i+1:]...)
			return true
		}
	}
	return false
}

// GetAccountByID returns an account by its ID.
func (c *Config) GetAccountByID(id string) *Account {
	for i := range c.Accounts {
		if c.Accounts[i].ID == id {
			return &c.Accounts[i]
		}
	}
	return nil
}

// GetAccountByEmail returns an account by its email address.
func (c *Config) GetAccountByEmail(email string) *Account {
	for i := range c.Accounts {
		if c.Accounts[i].Email == email {
			return &c.Accounts[i]
		}
	}
	return nil
}

// HasAccounts returns true if there are any configured accounts.
func (c *Config) HasAccounts() bool {
	return len(c.Accounts) > 0
}

// GetFirstAccount returns the first account or nil if none exist.
func (c *Config) GetFirstAccount() *Account {
	if len(c.Accounts) > 0 {
		return &c.Accounts[0]
	}
	return nil
}

// EnsurePGPDir creates the PGP keys directory if it doesn't exist.
func EnsurePGPDir() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	pgpDir := filepath.Join(dir, "pgp")
	return os.MkdirAll(pgpDir, 0700)
}
