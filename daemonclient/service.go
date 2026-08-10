package daemonclient

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/floatpane/matcha/backend"
	_ "github.com/floatpane/matcha/backend/imap"    // register imap backend for directService
	_ "github.com/floatpane/matcha/backend/jmap"    // register jmap backend for directService
	_ "github.com/floatpane/matcha/backend/maildir" // register maildir backend for directService
	_ "github.com/floatpane/matcha/backend/pop3"    // register pop3 backend for directService
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonrpc"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/loglevel"
	"github.com/floatpane/matcha/sender"
)

// Service abstracts daemon-backed vs direct email operations.
// TUI and CLI use this interface — they don't care which mode is active.
type Service interface {
	FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error)
	// FetchEmailBody returns body, MIME type ("text/html"|"text/plain"|""),
	// attachments, and any error.
	FetchEmailBody(accountID, folder string, uid uint32) (string, string, []backend.Attachment, error)
	DeleteEmails(accountID, folder string, uids []uint32) error
	ArchiveEmails(accountID, folder string, uids []uint32) error
	MoveEmails(accountID string, uids []uint32, src, dst string) error
	MarkRead(accountID, folder string, uids []uint32) error
	MarkUnread(accountID, folder string, uids []uint32) error
	QueueEmail(accountID string, to, cc, bcc []string, subject, body, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME, encryptSMIME, signPGP, encryptPGP bool, delaySeconds int, prebuiltRaw []byte) (string, error)
	CancelEmail(jobID string) error
	FetchFolders(accountID string) ([]backend.Folder, error)
	RefreshFolder(accountID, folder string) error
	Subscribe(accountID, folder string) error
	Unsubscribe(accountID, folder string) error
	ReloadConfig() error
	Events() <-chan *daemonrpc.Event
	IsDaemon() bool
	Close() error
}

// NewService connects to the daemon, auto-starting it if needed.
// Falls back to direct mode only if daemon cannot be started, or if DisableDaemon is set.
func NewService(cfg *config.Config) Service {
	if cfg.DisableDaemon {
		log.Println("service: daemon disabled by config, using direct mode")
		return newDirectService(cfg)
	}

	// Try connecting to existing daemon.
	if svc := tryConnectWithConfig(cfg); svc != nil {
		return svc
	}

	// Daemon not running — auto-start it.
	loglevel.Debugf("service: daemon not running, auto-starting")
	if err := autoStartDaemon(); err != nil {
		loglevel.Debugf("service: auto-start failed: %v, using direct mode", err)
		return newDirectService(cfg)
	}

	// Wait briefly for daemon to become ready, then connect.
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if svc := tryConnectWithConfig(cfg); svc != nil {
			loglevel.Debugf("service: connected to auto-started daemon")
			return svc
		}
	}

	log.Println("service: daemon started but not responding, using direct mode")
	return newDirectService(cfg)
}

func tryConnectWithConfig(cfg *config.Config) *daemonService {
	client, err := Dial()
	if err != nil {
		return nil
	}
	if err := client.Ping(); err != nil {
		client.Close() //nolint:errcheck,gosec
		return nil
	}
	return &daemonService{cfg: cfg, client: client}
}

func autoStartDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "daemon", "run") //nolint:noctx
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = DaemonProcAttr()

	return cmd.Start()
}

// daemonService routes all operations through the daemon socket.
type daemonService struct {
	cfg    *config.Config
	client *Client
	mu     sync.Mutex // guards client during reconnect
}

// call executes fn against the current client. If the call fails with a
// connection-level error, it attempts to reconnect once and retries.
func (s *daemonService) call(fn func(*Client) error) error {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	if client == nil {
		if !s.reconnect() {
			return fmt.Errorf("daemon connection lost and could not be re-established")
		}
		s.mu.Lock()
		client = s.client
		s.mu.Unlock()
	}

	err := fn(client)
	if err == nil {
		return nil
	}

	if !isConnError(err) {
		return err
	}

	loglevel.Debugf("service: daemon connection lost (%v), attempting reconnect", err)
	if s.reconnect() {
		s.mu.Lock()
		client = s.client
		s.mu.Unlock()
		err2 := fn(client)
		if err2 == nil {
			loglevel.Debugf("service: reconnected to daemon successfully")
			return nil
		}
		return err2
	}

	return err
}

// reconnect closes the old client and establishes a new connection,
// auto-starting the daemon if necessary. Returns true on success.
func (s *daemonService) reconnect() bool {
	s.mu.Lock()
	if s.client != nil {
		s.client.Close() //nolint:errcheck,gosec
		s.client = nil
	}
	s.mu.Unlock()

	if svc := tryConnectWithConfig(s.cfg); svc != nil {
		s.mu.Lock()
		s.client = svc.client
		s.mu.Unlock()
		return true
	}
	return false
}

// isConnError reports whether err indicates a broken socket connection
// rather than an application-level RPC error.
func isConnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "connection closed") {
		return true
	}
	if strings.Contains(msg, "use of closed network connection") {
		return true
	}
	if strings.Contains(msg, "broken pipe") {
		return true
	}
	if strings.Contains(msg, "EOF") {
		return true
	}
	if strings.Contains(msg, "connect to daemon") {
		return true
	}
	return false
}

func (s *daemonService) FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error) {
	var emails []backend.Email
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodFetchEmails, daemonrpc.FetchEmailsParams{
			AccountID: accountID,
			Folder:    folder,
			Limit:     limit,
			Offset:    offset,
		}, &emails)
	})
	return emails, err
}

func (s *daemonService) FetchEmailBody(accountID, folder string, uid uint32) (string, string, []backend.Attachment, error) {
	var result daemonrpc.FetchEmailBodyResult
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodFetchEmailBody, daemonrpc.FetchEmailBodyParams{
			AccountID: accountID,
			Folder:    folder,
			UID:       uid,
		}, &result)
	})
	if err != nil {
		return "", "", nil, err
	}

	var attachments []backend.Attachment
	for _, a := range result.Attachments {
		attachments = append(attachments, backend.Attachment{
			Filename: a.Filename,
			PartID:   a.PartID,
			Encoding: a.Encoding,
			MIMEType: a.MIMEType,
		})
	}
	return result.Body, result.BodyMIMEType, attachments, nil
}

func (s *daemonService) DeleteEmails(accountID, folder string, uids []uint32) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodDeleteEmails, daemonrpc.DeleteEmailsParams{
			AccountID: accountID,
			Folder:    folder,
			UIDs:      uids,
		}, nil)
	})
}

func (s *daemonService) ArchiveEmails(accountID, folder string, uids []uint32) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodArchiveEmails, daemonrpc.ArchiveEmailsParams{
			AccountID: accountID,
			Folder:    folder,
			UIDs:      uids,
		}, nil)
	})
}

func (s *daemonService) MoveEmails(accountID string, uids []uint32, src, dst string) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodMoveEmails, daemonrpc.MoveEmailsParams{
			AccountID:    accountID,
			UIDs:         uids,
			SourceFolder: src,
			DestFolder:   dst,
		}, nil)
	})
}

func (s *daemonService) MarkRead(accountID, folder string, uids []uint32) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodMarkRead, daemonrpc.MarkReadParams{
			AccountID: accountID,
			Folder:    folder,
			UIDs:      uids,
			Read:      true,
		}, nil)
	})
}

func (s *daemonService) MarkUnread(accountID, folder string, uids []uint32) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodMarkRead, daemonrpc.MarkReadParams{
			AccountID: accountID,
			Folder:    folder,
			UIDs:      uids,
			Read:      false,
		}, nil)
	})
}

func (s *daemonService) QueueEmail(accountID string, to, cc, bcc []string, subject, body, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME, encryptSMIME, signPGP, encryptPGP bool, delaySeconds int, prebuiltRaw []byte) (string, error) {
	var result daemonrpc.QueueEmailResult
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodQueueEmail, daemonrpc.QueueEmailParams{
			Email: daemonrpc.SendEmailParams{
				AccountID:    accountID,
				To:           to,
				Cc:           cc,
				Bcc:          bcc,
				Subject:      subject,
				Body:         body,
				HTMLBody:     htmlBody,
				Images:       images,
				Attachments:  attachments,
				InReplyTo:    inReplyTo,
				References:   references,
				SignSMIME:    signSMIME,
				EncryptSMIME: encryptSMIME,
				SignPGP:      signPGP,
				EncryptPGP:   encryptPGP,
				PrebuiltRaw:  prebuiltRaw,
			},
			DelaySeconds: delaySeconds,
		}, &result)
	})
	return result.JobID, err
}

func (s *daemonService) CancelEmail(jobID string) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodCancelEmail, daemonrpc.CancelEmailParams{
			JobID: jobID,
		}, nil)
	})
}

func (s *daemonService) FetchFolders(accountID string) ([]backend.Folder, error) {
	var folders []backend.Folder
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodFetchFolders, daemonrpc.FetchFoldersParams{
			AccountID: accountID,
		}, &folders)
	})
	return folders, err
}

func (s *daemonService) RefreshFolder(accountID, folder string) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodRefreshFolder, daemonrpc.RefreshFolderParams{
			AccountID: accountID,
			Folder:    folder,
		}, nil)
	})
}

func (s *daemonService) Subscribe(accountID, folder string) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodSubscribe, daemonrpc.SubscribeParams{
			AccountID: accountID,
			Folder:    folder,
		}, nil)
	})
}

func (s *daemonService) Unsubscribe(accountID, folder string) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodUnsubscribe, daemonrpc.UnsubscribeParams{
			AccountID: accountID,
			Folder:    folder,
		}, nil)
	})
}

func (s *daemonService) ReloadConfig() error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodReloadConfig, nil, nil)
	})
}

func (s *daemonService) Events() <-chan *daemonrpc.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		ch := make(chan *daemonrpc.Event)
		close(ch)
		return ch
	}
	return s.client.Events()
}

func (s *daemonService) IsDaemon() bool { return true }

func (s *daemonService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// directService runs operations in-process (no daemon).
// This is the fallback when daemon is not running.
type directService struct {
	cfg       *config.Config
	providers map[string]backend.Provider
	events    chan *daemonrpc.Event
}

func newDirectService(cfg *config.Config) *directService {
	s := &directService{
		cfg:       cfg,
		providers: make(map[string]backend.Provider),
		events:    make(chan *daemonrpc.Event, 64),
	}
	s.initProviders()
	return s
}

func (s *directService) initProviders() {
	for i := range s.cfg.Accounts {
		acct := &s.cfg.Accounts[i]
		if _, ok := s.providers[acct.ID]; ok {
			continue
		}
		p, err := backend.New(acct)
		if err != nil {
			log.Printf("direct service: provider for %s failed: %v", acct.Email, err)
			continue
		}
		s.providers[acct.ID] = p
	}
}

func (s *directService) getProvider(accountID string) (backend.Provider, error) {
	p, ok := s.providers[accountID]
	if !ok {
		return nil, &daemonrpc.Error{Code: daemonrpc.ErrCodeInternal, Message: "no provider for account " + accountID}
	}
	return p, nil
}

func (s *directService) FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error) {
	p, err := s.getProvider(accountID)
	if err != nil {
		return nil, err
	}
	return p.FetchEmails(context.Background(), folder, limit, offset)
}

func (s *directService) FetchEmailBody(accountID, folder string, uid uint32) (string, string, []backend.Attachment, error) {
	p, err := s.getProvider(accountID)
	if err != nil {
		return "", "", nil, err
	}
	return p.FetchEmailBody(context.Background(), folder, uid)
}

func (s *directService) DeleteEmails(accountID, folder string, uids []uint32) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	return p.DeleteEmails(context.Background(), folder, uids)
}

func (s *directService) ArchiveEmails(accountID, folder string, uids []uint32) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	return p.ArchiveEmails(context.Background(), folder, uids)
}

func (s *directService) MoveEmails(accountID string, uids []uint32, src, dst string) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	return p.MoveEmails(context.Background(), uids, src, dst)
}

func (s *directService) MarkRead(accountID, folder string, uids []uint32) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	for _, uid := range uids {
		if err := p.MarkAsRead(context.Background(), folder, uid); err != nil {
			return err
		}
	}
	return nil
}

func (s *directService) MarkUnread(accountID, folder string, uids []uint32) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	for _, uid := range uids {
		if err := p.MarkAsUnread(context.Background(), folder, uid); err != nil {
			return err
		}
	}
	return nil
}

func (s *directService) FetchFolders(accountID string) ([]backend.Folder, error) {
	p, err := s.getProvider(accountID)
	if err != nil {
		return nil, err
	}
	return p.FetchFolders(context.Background())
}

func (s *directService) RefreshFolder(_, _ string) error {
	// In direct mode, caller handles refresh via their own fetcher calls.
	return nil
}

func (s *directService) Subscribe(_, _ string) error {
	// No-op in direct mode — TUI manages its own IDLE.
	return nil
}

func (s *directService) Unsubscribe(_, _ string) error {
	return nil
}

func (s *directService) ReloadConfig() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	s.cfg = cfg
	s.initProviders()
	return nil
}

func (s *directService) Events() <-chan *daemonrpc.Event {
	return s.events
}

func (s *directService) IsDaemon() bool { return false }

func (s *directService) Close() error {
	for _, p := range s.providers {
		p.Close() //nolint:errcheck,gosec
	}
	close(s.events)
	return nil
}

func (s *directService) QueueEmail(accountID string, to, cc, bcc []string, subject, body, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME, encryptSMIME, signPGP, encryptPGP bool, _ int, prebuiltRaw []byte) (string, error) {
	acct := s.cfg.GetAccountByID(accountID)
	if acct == nil {
		return "", fmt.Errorf("no account for %s", accountID)
	}

	var rawMsg []byte
	if len(prebuiltRaw) > 0 {
		allRecipients := append(append([]string{}, to...), append(cc, bcc...)...)
		if err := sender.DeliverRaw(acct, allRecipients, prebuiltRaw); err != nil {
			return "", err
		}
		rawMsg = prebuiltRaw
	} else {
		var err error
		rawMsg, err = sender.SendEmail(
			acct,
			to,
			cc,
			bcc,
			subject,
			body,
			htmlBody,
			images,
			attachments,
			inReplyTo,
			references,
			signSMIME,
			encryptSMIME,
			signPGP,
			encryptPGP,
		)
		if err != nil {
			return "", err
		}
	}

	if acct.ServiceProvider != "gmail" {
		if err := fetcher.AppendToSentMailbox(acct, rawMsg); err != nil {
			log.Printf("direct: append to sent failed: %v", err)
		}
	}

	return "", nil
}

func (s *directService) CancelEmail(_ string) error {
	return nil
}
