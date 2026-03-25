package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

const keyringServiceName = "matcha-email-client"

// Account stores the configuration for a single email account.
type Account struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Password        string `json:"-"`                // "-" prevents the password from being saved to config.json
	ServiceProvider string `json:"service_provider"` // "gmail", "icloud", or "custom"
	// FetchEmail is the single email address for which messages should be fetched.
	// If empty, it will default to `Email` when accounts are added.
	FetchEmail string `json:"fetch_email,omitempty"`

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

	// OAuth2 settings
	AuthMethod string `json:"auth_method,omitempty"` // "password" (default) or "oauth2"
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
}

// GetIMAPServer returns the IMAP server address for the account.
func (a *Account) GetIMAPServer() string {
	switch a.ServiceProvider {
	case "gmail":
		return "imap.gmail.com"
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
	case "gmail", "icloud":
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
	case "gmail", "icloud":
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

// configFile returns the full path to the configuration file.
func configFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// SaveConfig saves the given configuration to the config file and passwords to the keyring.
func SaveConfig(config *Config) error {
	// Save passwords to the OS keyring before writing the JSON file
	for _, acc := range config.Accounts {
		if acc.Password != "" {
			// We ignore the error here because some environments (like headless CI)
			// might not have a keyring service, but we still want to save the rest of the config.
			_ = keyring.Set(keyringServiceName, acc.Email, acc.Password)
		}
	}

	path, err := configFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadConfig loads the configuration from the config file and passwords from the keyring.
// It automatically migrates plain-text passwords to the OS keyring if they exist.
func LoadConfig() (*Config, error) {
	path, err := configFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	var needsMigration bool

	type rawAccount struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		Email              string `json:"email"`
		Password           string `json:"password,omitempty"`
		ServiceProvider    string `json:"service_provider"`
		FetchEmail         string `json:"fetch_email,omitempty"`
		IMAPServer         string `json:"imap_server,omitempty"`
		IMAPPort           int    `json:"imap_port,omitempty"`
		SMTPServer         string `json:"smtp_server,omitempty"`
		SMTPPort           int    `json:"smtp_port,omitempty"`
		Insecure           bool   `json:"insecure,omitempty"`
		SMIMECert          string `json:"smime_cert,omitempty"`
		SMIMEKey           string `json:"smime_key,omitempty"`
		SMIMESignByDefault bool   `json:"smime_sign_by_default,omitempty"`
		AuthMethod         string `json:"auth_method,omitempty"`
	}
	type diskConfig struct {
		Accounts             []rawAccount  `json:"accounts"`
		DisableImages        bool          `json:"disable_images,omitempty"`
		HideTips             bool          `json:"hide_tips,omitempty"`
		DisableNotifications bool          `json:"disable_notifications,omitempty"`
		Theme                string        `json:"theme,omitempty"`
		MailingLists         []MailingList `json:"mailing_lists,omitempty"`
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
	for _, rawAcc := range raw.Accounts {
		acc := Account{
			ID:                 rawAcc.ID,
			Name:               rawAcc.Name,
			Email:              rawAcc.Email,
			ServiceProvider:    rawAcc.ServiceProvider,
			FetchEmail:         rawAcc.FetchEmail,
			IMAPServer:         rawAcc.IMAPServer,
			IMAPPort:           rawAcc.IMAPPort,
			SMTPServer:         rawAcc.SMTPServer,
			SMTPPort:           rawAcc.SMTPPort,
			Insecure:           rawAcc.Insecure,
			SMIMECert:          rawAcc.SMIMECert,
			SMIMEKey:           rawAcc.SMIMEKey,
			SMIMESignByDefault: rawAcc.SMIMESignByDefault,
			AuthMethod:         rawAcc.AuthMethod,
		}

		if rawAcc.Password != "" {
			// Found a plain-text password! Move it to the OS Keyring.
			_ = keyring.Set(keyringServiceName, rawAcc.Email, rawAcc.Password)
			acc.Password = rawAcc.Password
			needsMigration = true
		} else {
			// No plaintext password in JSON, fetch from Keyring as normal.
			if pwd, err := keyring.Get(keyringServiceName, acc.Email); err == nil {
				acc.Password = pwd
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
			// Delete password from OS Keyring when account is removed
			_ = keyring.Delete(keyringServiceName, acc.Email)

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
