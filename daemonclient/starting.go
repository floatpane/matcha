package daemonclient

import (
	"sync"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonrpc"
)

// StartingService is a Service whose connection is established in the
// background, so that creating it never blocks the UI: NewService may spend
// seconds connecting to (or auto-starting) the daemon.
//
// Operations wait until the real service is ready; they are meant to be
// called from Bubble Tea commands, which run off the UI goroutine. UI code
// must not block on it: IsReady, IsDaemon and Events never wait, and Close
// returns immediately.
type StartingService struct {
	ready chan struct{}

	mu     sync.Mutex
	svc    Service
	closed bool
	// closeDone is closed once the underlying service has been closed.
	closeDone chan struct{}
}

var _ Service = (*StartingService)(nil)

// StartService begins connecting with NewService in the background and
// returns at once. It works on a snapshot of cfg, which the UI may keep
// editing.
func StartService(cfg *config.Config) *StartingService {
	snapshot := snapshotConfig(cfg)
	return StartServiceFunc(func() Service { return NewService(snapshot) })
}

// StartServiceFunc runs connect in the background and returns a
// StartingService for its result.
func StartServiceFunc(connect func() Service) *StartingService {
	s := &StartingService{ready: make(chan struct{}), closeDone: make(chan struct{})}
	go func() {
		svc := connect()
		s.mu.Lock()
		s.svc = svc
		closed := s.closed
		s.mu.Unlock()
		if closed {
			// Closed while starting. Operations still waiting run against
			// the closed service (as after any Close), never a live one.
			_ = svc.Close()
			close(s.closeDone)
		}
		close(s.ready)
	}()
	return s
}

// Ready is closed once the underlying service exists.
func (s *StartingService) Ready() <-chan struct{} { return s.ready }

// IsReady reports whether the underlying service exists, without waiting.
func (s *StartingService) IsReady() bool {
	select {
	case <-s.ready:
		return true
	default:
		return false
	}
}

// wait blocks until the underlying service exists and, after Close, until
// it is closed: services handle calls made after Close (the daemon client
// uses a one-off connection), but not calls racing their shutdown.
func (s *StartingService) wait() Service {
	<-s.ready
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		<-s.closeDone
	}
	return s.svc
}

func (s *StartingService) FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error) {
	return s.wait().FetchEmails(accountID, folder, limit, offset)
}

func (s *StartingService) FetchEmailBody(accountID, folder string, uid uint32) (string, string, []backend.Attachment, error) {
	return s.wait().FetchEmailBody(accountID, folder, uid)
}

func (s *StartingService) DeleteEmails(accountID, folder string, uids []uint32) error {
	return s.wait().DeleteEmails(accountID, folder, uids)
}

func (s *StartingService) ArchiveEmails(accountID, folder string, uids []uint32) error {
	return s.wait().ArchiveEmails(accountID, folder, uids)
}

func (s *StartingService) MoveEmails(accountID string, uids []uint32, src, dst string) error {
	return s.wait().MoveEmails(accountID, uids, src, dst)
}

func (s *StartingService) MarkRead(accountID, folder string, uids []uint32) error {
	return s.wait().MarkRead(accountID, folder, uids)
}

func (s *StartingService) MarkUnread(accountID, folder string, uids []uint32) error {
	return s.wait().MarkUnread(accountID, folder, uids)
}

func (s *StartingService) QueueEmail(accountID string, to, cc, bcc []string, subject, body, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME, encryptSMIME, signPGP, encryptPGP bool, delaySeconds int, prebuiltRaw []byte) (string, error) {
	return s.wait().QueueEmail(accountID, to, cc, bcc, subject, body, htmlBody, images, attachments, inReplyTo, references, signSMIME, encryptSMIME, signPGP, encryptPGP, delaySeconds, prebuiltRaw)
}

func (s *StartingService) CancelEmail(jobID string) error {
	return s.wait().CancelEmail(jobID)
}

func (s *StartingService) FetchFolders(accountID string) ([]backend.Folder, error) {
	return s.wait().FetchFolders(accountID)
}

func (s *StartingService) RefreshFolder(accountID, folder string) error {
	return s.wait().RefreshFolder(accountID, folder)
}

func (s *StartingService) Subscribe(accountID, folder string) error {
	return s.wait().Subscribe(accountID, folder)
}

func (s *StartingService) Unsubscribe(accountID, folder string) error {
	return s.wait().Unsubscribe(accountID, folder)
}

func (s *StartingService) Search(accountID, folder string, query backend.SearchQuery) ([]backend.Email, error) {
	return s.wait().Search(accountID, folder, query)
}

func (s *StartingService) ReloadConfig() error {
	return s.wait().ReloadConfig()
}

// Events returns the underlying event channel, or nil while starting. Wait
// for Ready before listening.
func (s *StartingService) Events() <-chan *daemonrpc.Event {
	if !s.IsReady() {
		return nil
	}
	return s.svc.Events()
}

// IsDaemon reports whether the ready service talks to the daemon. It is
// false while starting.
func (s *StartingService) IsDaemon() bool {
	return s.IsReady() && s.svc.IsDaemon()
}

// Close shuts the service down without blocking the caller: a service that
// is still starting is closed as soon as it is ready, and closing a ready
// one (which may log out of IMAP servers) happens in the background.
func (s *StartingService) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	svc := s.svc
	s.mu.Unlock()
	if svc != nil {
		go func() {
			_ = svc.Close()
			close(s.closeDone)
		}()
	}
	return nil
}
