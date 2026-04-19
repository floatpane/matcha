package daemonclient

import (
	"context"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonrpc"
)

// Service abstracts daemon-backed vs direct email operations.
// TUI and CLI use this interface — they don't care which mode is active.
type Service interface {
	FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error)
	FetchEmailBody(accountID, folder string, uid uint32) (string, []backend.Attachment, error)
	DeleteEmails(accountID, folder string, uids []uint32) error
	ArchiveEmails(accountID, folder string, uids []uint32) error
	MoveEmails(accountID string, uids []uint32, src, dst string) error
	MarkRead(accountID, folder string, uids []uint32) error
	FetchFolders(accountID string) ([]backend.Folder, error)
	RefreshFolder(accountID, folder string) error
	Subscribe(accountID, folder string) error
	Unsubscribe(accountID, folder string) error
	Events() <-chan *daemonrpc.Event
	IsDaemon() bool
	Close() error
}

// NewService connects to the daemon, auto-starting it if needed.
// Falls back to direct mode only if daemon cannot be started.
func NewService(cfg *config.Config) Service {
	// Try connecting to existing daemon.
	if svc := tryConnect(); svc != nil {
		return svc
	}

	// Daemon not running — auto-start it.
	log.Println("service: daemon not running, auto-starting")
	if err := autoStartDaemon(); err != nil {
		log.Printf("service: auto-start failed: %v, using direct mode", err)
		return newDirectService(cfg)
	}

	// Wait briefly for daemon to become ready, then connect.
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if svc := tryConnect(); svc != nil {
			log.Println("service: connected to auto-started daemon")
			return svc
		}
	}

	log.Println("service: daemon started but not responding, using direct mode")
	return newDirectService(cfg)
}

func tryConnect() *daemonService {
	client, err := Dial()
	if err != nil {
		return nil
	}
	if err := client.Ping(); err != nil {
		client.Close()
		return nil
	}
	return &daemonService{client: client}
}

func autoStartDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "daemon", "run")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = DaemonProcAttr()

	return cmd.Start()
}

// daemonService routes all operations through the daemon socket.
type daemonService struct {
	client *Client
}

func (s *daemonService) FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error) {
	var emails []backend.Email
	err := s.client.Call(daemonrpc.MethodFetchEmails, daemonrpc.FetchEmailsParams{
		AccountID: accountID,
		Folder:    folder,
		Limit:     limit,
		Offset:    offset,
	}, &emails)
	return emails, err
}

func (s *daemonService) FetchEmailBody(accountID, folder string, uid uint32) (string, []backend.Attachment, error) {
	var result daemonrpc.FetchEmailBodyResult
	err := s.client.Call(daemonrpc.MethodFetchEmailBody, daemonrpc.FetchEmailBodyParams{
		AccountID: accountID,
		Folder:    folder,
		UID:       uid,
	}, &result)
	if err != nil {
		return "", nil, err
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
	return result.Body, attachments, nil
}

func (s *daemonService) DeleteEmails(accountID, folder string, uids []uint32) error {
	return s.client.Call(daemonrpc.MethodDeleteEmails, daemonrpc.DeleteEmailsParams{
		AccountID: accountID,
		Folder:    folder,
		UIDs:      uids,
	}, nil)
}

func (s *daemonService) ArchiveEmails(accountID, folder string, uids []uint32) error {
	return s.client.Call(daemonrpc.MethodArchiveEmails, daemonrpc.ArchiveEmailsParams{
		AccountID: accountID,
		Folder:    folder,
		UIDs:      uids,
	}, nil)
}

func (s *daemonService) MoveEmails(accountID string, uids []uint32, src, dst string) error {
	return s.client.Call(daemonrpc.MethodMoveEmails, daemonrpc.MoveEmailsParams{
		AccountID:    accountID,
		UIDs:         uids,
		SourceFolder: src,
		DestFolder:   dst,
	}, nil)
}

func (s *daemonService) MarkRead(accountID, folder string, uids []uint32) error {
	return s.client.Call(daemonrpc.MethodMarkRead, daemonrpc.MarkReadParams{
		AccountID: accountID,
		Folder:    folder,
		UIDs:      uids,
		Read:      true,
	}, nil)
}

func (s *daemonService) FetchFolders(accountID string) ([]backend.Folder, error) {
	var folders []backend.Folder
	err := s.client.Call(daemonrpc.MethodFetchFolders, daemonrpc.FetchFoldersParams{
		AccountID: accountID,
	}, &folders)
	return folders, err
}

func (s *daemonService) RefreshFolder(accountID, folder string) error {
	return s.client.Call(daemonrpc.MethodRefreshFolder, daemonrpc.RefreshFolderParams{
		AccountID: accountID,
		Folder:    folder,
	}, nil)
}

func (s *daemonService) Subscribe(accountID, folder string) error {
	return s.client.Call(daemonrpc.MethodSubscribe, daemonrpc.SubscribeParams{
		AccountID: accountID,
		Folder:    folder,
	}, nil)
}

func (s *daemonService) Unsubscribe(accountID, folder string) error {
	return s.client.Call(daemonrpc.MethodUnsubscribe, daemonrpc.UnsubscribeParams{
		AccountID: accountID,
		Folder:    folder,
	}, nil)
}

func (s *daemonService) Events() <-chan *daemonrpc.Event {
	return s.client.Events()
}

func (s *daemonService) IsDaemon() bool { return true }

func (s *daemonService) Close() error {
	return s.client.Close()
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

func (s *directService) FetchEmailBody(accountID, folder string, uid uint32) (string, []backend.Attachment, error) {
	p, err := s.getProvider(accountID)
	if err != nil {
		return "", nil, err
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

func (s *directService) Events() <-chan *daemonrpc.Event {
	return s.events
}

func (s *directService) IsDaemon() bool { return false }

func (s *directService) Close() error {
	for _, p := range s.providers {
		p.Close()
	}
	close(s.events)
	return nil
}
