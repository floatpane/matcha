package emailactions

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/emailstore"
	"github.com/floatpane/matcha/tui"
)

const (
	actionKindDelete  = "delete"
	actionKindArchive = "archive"
	actionKindMove    = "move"
)

// Dependencies provides the runtime services required by the email action
// manager.
type Dependencies struct {
	Service daemonclient.Service
	Config  *config.Config
}

// FolderInboxView is the subset of *tui.FolderInbox used by the action manager.
type FolderInboxView interface {
	SetEmails(emails []fetcher.Email, accounts []config.Account)
	IncrementUnreadCount(folder string)
	DecrementUnreadCount(folder string)
	GetFolders() []string
	GetUnreadCountsCopy() map[string]int
}

// Manager handles delete, archive, and move actions with an undo grace period.
type Manager struct {
	deps          Dependencies
	store         *emailstore.Store
	folderInbox   FolderInboxView
	pendingAction *emailstore.PendingAction
	ActionNotice  string
}

func NewManager(deps Dependencies, store *emailstore.Store, folderInbox FolderInboxView) *Manager {
	return &Manager{
		deps:        deps,
		store:       store,
		folderInbox: folderInbox,
	}
}

func (m *Manager) PendingAction() *emailstore.PendingAction {
	return m.pendingAction
}

func (m *Manager) IsPending() bool {
	return m.pendingAction != nil
}

func (m *Manager) StartActionGracePeriod(pa *emailstore.PendingAction, notice string) tea.Cmd {
	m.pendingAction = pa
	m.ActionNotice = notice
	delay := time.Duration(m.deps.Config.GetUndoDelaySeconds()) * time.Second
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return tui.ActionGracePeriodExpiredMsg{JobID: pa.JobID}
	})
}

func (m *Manager) FlushPendingAction() tea.Cmd {
	if m.pendingAction == nil {
		return nil
	}
	pa := m.pendingAction
	m.pendingAction = nil
	m.ActionNotice = ""
	return m.ExecutePendingAction(pa)
}

func (m *Manager) ExecutePendingAction(pa *emailstore.PendingAction) tea.Cmd {
	switch pa.Kind {
	case actionKindDelete:
		return m.BatchDeleteEmailsCmd(pa.UIDs, pa.AccountID, pa.FolderName, pa.Mailbox, len(pa.UIDs))
	case actionKindArchive:
		return m.BatchArchiveEmailsCmd(pa.UIDs, pa.AccountID, pa.FolderName, pa.Mailbox, len(pa.UIDs))
	case actionKindMove:
		return m.BatchMoveEmailsCmd(pa.UIDs, pa.AccountID, pa.FolderName, pa.DestFolder, len(pa.UIDs))
	}
	return nil
}

func (m *Manager) RestorePendingAction() {
	if m.pendingAction == nil {
		return
	}
	pa := m.pendingAction
	m.pendingAction = nil
	m.ActionNotice = ""

	m.store.Emails = pa.EmailsSnap
	m.store.EmailsByAcct[pa.AccountID] = pa.AcctSnap
	m.store.FolderEmails[pa.FolderName] = pa.FolderSnap

	if m.folderInbox != nil {
		m.folderInbox.SetEmails(pa.FolderSnap, m.deps.Config.Accounts)
		restored := false
		for _, uid := range pa.UIDs {
			for _, e := range pa.FolderSnap {
				if e.UID == uid && e.AccountID == pa.AccountID {
					if !e.IsRead {
						m.folderInbox.IncrementUnreadCount(pa.FolderName)
						restored = true
					}
					break
				}
			}
		}
		if restored {
			config.SaveAccountFolders(pa.AccountID, m.folderInbox.GetFolders(), m.folderInbox.GetUnreadCountsCopy()) //nolint:errcheck,gosec
		}
	}
	go emailstore.SaveFolderEmailsToCache(pa.FolderName, pa.FolderSnap)
}

func (m *Manager) OnGracePeriodExpired(msg tui.ActionGracePeriodExpiredMsg) tea.Cmd {
	if m.pendingAction != nil && m.pendingAction.JobID == msg.JobID {
		pa := m.pendingAction
		m.pendingAction = nil
		m.ActionNotice = ""
		return m.ExecutePendingAction(pa)
	}
	return nil
}

func (m *Manager) HandleDeleteEmailMsg(msg tui.DeleteEmailMsg, folderName string) (*emailstore.PendingAction, string) {
	emailsSnap := m.store.CloneEmails()
	acctSnap := m.store.CloneAccount(msg.AccountID)
	folderSnap := m.store.CloneFolder(folderName)

	m.decrementFolderUnreadForRemoved(folderName, msg.AccountID, []uint32{msg.UID})
	m.store.RemoveEmailFromStores(msg.UID, msg.AccountID)
	m.store.RemoveFolderEmail(folderName, msg.UID, msg.AccountID)

	pa := emailstore.NewPendingAction(
		actionKindDelete,
		msg.AccountID,
		folderName,
		"",
		[]uint32{msg.UID},
		msg.Mailbox,
		emailsSnap,
		acctSnap,
		folderSnap,
	)
	notice := fmt.Sprintf("Email deleted (%s to undo)", config.Keybinds.Composer.UndoSend)
	return pa, notice
}

func (m *Manager) HandleArchiveEmailMsg(msg tui.ArchiveEmailMsg, folderName string) (*emailstore.PendingAction, string) {
	emailsSnap := m.store.CloneEmails()
	acctSnap := m.store.CloneAccount(msg.AccountID)
	folderSnap := m.store.CloneFolder(folderName)

	m.decrementFolderUnreadForRemoved(folderName, msg.AccountID, []uint32{msg.UID})
	m.store.RemoveEmailFromStores(msg.UID, msg.AccountID)
	m.store.RemoveFolderEmail(folderName, msg.UID, msg.AccountID)

	pa := emailstore.NewPendingAction(
		actionKindArchive,
		msg.AccountID,
		folderName,
		"",
		[]uint32{msg.UID},
		msg.Mailbox,
		emailsSnap,
		acctSnap,
		folderSnap,
	)
	notice := fmt.Sprintf("Email archived (%s to undo)", config.Keybinds.Composer.UndoSend)
	return pa, notice
}

func (m *Manager) HandleMoveEmailMsg(msg tui.MoveEmailToFolderMsg, folderName string) (*emailstore.PendingAction, string) {
	emailsSnap := m.store.CloneEmails()
	acctSnap := m.store.CloneAccount(msg.AccountID)
	folderSnap := m.store.CloneFolder(folderName)

	m.store.RemoveEmailFromStores(msg.UID, msg.AccountID)
	m.store.RemoveFolderEmail(folderName, msg.UID, msg.AccountID)

	pa := emailstore.NewPendingAction(
		actionKindMove,
		msg.AccountID,
		folderName,
		msg.DestFolder,
		[]uint32{msg.UID},
		tui.MailboxKind(""),
		emailsSnap,
		acctSnap,
		folderSnap,
	)
	notice := fmt.Sprintf("Email moved to %s (%s to undo)", msg.DestFolder, config.Keybinds.Composer.UndoSend)
	return pa, notice
}

func (m *Manager) HandleBatchDeleteEmailsMsg(msg tui.BatchDeleteEmailsMsg, folderName string) (*emailstore.PendingAction, string) {
	emailsSnap := m.store.CloneEmails()
	acctSnap := m.store.CloneAccount(msg.AccountID)
	folderSnap := m.store.CloneFolder(folderName)

	m.decrementFolderUnreadForRemoved(folderName, msg.AccountID, msg.UIDs)
	for _, uid := range msg.UIDs {
		m.store.RemoveEmailFromStores(uid, msg.AccountID)
	}
	m.store.RemoveFolderEmails(folderName, msg.AccountID, msg.UIDs)

	pa := emailstore.NewPendingAction(
		actionKindDelete,
		msg.AccountID,
		folderName,
		"",
		msg.UIDs,
		msg.Mailbox,
		emailsSnap,
		acctSnap,
		folderSnap,
	)
	notice := fmt.Sprintf("%d emails deleted (%s to undo)", len(msg.UIDs), config.Keybinds.Composer.UndoSend)
	if len(msg.UIDs) == 1 {
		notice = fmt.Sprintf("Email deleted (%s to undo)", config.Keybinds.Composer.UndoSend)
	}
	return pa, notice
}

func (m *Manager) HandleBatchArchiveEmailsMsg(msg tui.BatchArchiveEmailsMsg, folderName string) (*emailstore.PendingAction, string) {
	emailsSnap := m.store.CloneEmails()
	acctSnap := m.store.CloneAccount(msg.AccountID)
	folderSnap := m.store.CloneFolder(folderName)

	m.decrementFolderUnreadForRemoved(folderName, msg.AccountID, msg.UIDs)
	for _, uid := range msg.UIDs {
		m.store.RemoveEmailFromStores(uid, msg.AccountID)
	}
	m.store.RemoveFolderEmails(folderName, msg.AccountID, msg.UIDs)

	pa := emailstore.NewPendingAction(
		actionKindArchive,
		msg.AccountID,
		folderName,
		"",
		msg.UIDs,
		msg.Mailbox,
		emailsSnap,
		acctSnap,
		folderSnap,
	)
	notice := fmt.Sprintf("%d emails archived (%s to undo)", len(msg.UIDs), config.Keybinds.Composer.UndoSend)
	if len(msg.UIDs) == 1 {
		notice = fmt.Sprintf("Email archived (%s to undo)", config.Keybinds.Composer.UndoSend)
	}
	return pa, notice
}

func (m *Manager) HandleBatchMoveEmailsMsg(msg tui.BatchMoveEmailsMsg, folderName string) (*emailstore.PendingAction, string) {
	emailsSnap := m.store.CloneEmails()
	acctSnap := m.store.CloneAccount(msg.AccountID)
	folderSnap := m.store.CloneFolder(folderName)

	for _, uid := range msg.UIDs {
		m.store.RemoveEmailFromStores(uid, msg.AccountID)
	}
	m.store.RemoveFolderEmails(folderName, msg.AccountID, msg.UIDs)

	pa := emailstore.NewPendingAction(
		actionKindMove,
		msg.AccountID,
		folderName,
		msg.DestFolder,
		msg.UIDs,
		tui.MailboxKind(""),
		emailsSnap,
		acctSnap,
		folderSnap,
	)
	notice := fmt.Sprintf("%d emails moved to %s (%s to undo)", len(msg.UIDs), msg.DestFolder, config.Keybinds.Composer.UndoSend)
	if len(msg.UIDs) == 1 {
		notice = fmt.Sprintf("Email moved to %s (%s to undo)", msg.DestFolder, config.Keybinds.Composer.UndoSend)
	}
	return pa, notice
}

func (m *Manager) BatchDeleteEmailsCmd(uids []uint32, accountID, folderName string, mailbox tui.MailboxKind, count int) tea.Cmd {
	return func() tea.Msg {
		if m.deps.Service == nil {
			return tui.BatchEmailActionDoneMsg{
				Count:        count,
				SuccessCount: 0,
				FailureCount: count,
				Action:       actionKindDelete,
				Mailbox:      mailbox,
				Err:          fmt.Errorf("service not initialized"),
			}
		}

		err := m.deps.Service.DeleteEmails(accountID, folderName, uids)
		successCount, failureCount := count, 0
		if err != nil {
			successCount, failureCount = 0, count
		}

		return tui.BatchEmailActionDoneMsg{
			Count:        count,
			SuccessCount: successCount,
			FailureCount: failureCount,
			Action:       actionKindDelete,
			Mailbox:      mailbox,
			Err:          err,
		}
	}
}

func (m *Manager) BatchArchiveEmailsCmd(uids []uint32, accountID, folderName string, mailbox tui.MailboxKind, count int) tea.Cmd {
	return func() tea.Msg {
		if m.deps.Service == nil {
			return tui.BatchEmailActionDoneMsg{
				Count:        count,
				SuccessCount: 0,
				FailureCount: count,
				Action:       actionKindArchive,
				Mailbox:      mailbox,
				Err:          fmt.Errorf("service not initialized"),
			}
		}

		err := m.deps.Service.ArchiveEmails(accountID, folderName, uids)
		successCount, failureCount := count, 0
		if err != nil {
			successCount, failureCount = 0, count
		}

		return tui.BatchEmailActionDoneMsg{
			Count:        count,
			SuccessCount: successCount,
			FailureCount: failureCount,
			Action:       actionKindArchive,
			Mailbox:      mailbox,
			Err:          err,
		}
	}
}

func (m *Manager) BatchMoveEmailsCmd(uids []uint32, accountID, sourceFolder, destFolder string, count int) tea.Cmd {
	return func() tea.Msg {
		if m.deps.Service == nil {
			return tui.BatchEmailActionDoneMsg{
				Count:        count,
				SuccessCount: 0,
				FailureCount: count,
				Action:       actionKindMove,
				Err:          fmt.Errorf("service not initialized"),
			}
		}

		err := m.deps.Service.MoveEmails(accountID, uids, sourceFolder, destFolder)
		successCount, failureCount := count, 0
		if err != nil {
			successCount, failureCount = 0, count
		}

		return tui.BatchEmailActionDoneMsg{
			Count:        count,
			SuccessCount: successCount,
			FailureCount: failureCount,
			Action:       actionKindMove,
			Err:          err,
		}
	}
}

func (m *Manager) decrementFolderUnreadForRemoved(folderName, accountID string, uids []uint32) {
	if m.folderInbox == nil || len(uids) == 0 || m.store == nil {
		return
	}

	emails, ok := m.store.FolderEmails[folderName]
	if !ok {
		return
	}

	decremented := false
	for _, uid := range uids {
		for _, e := range emails {
			if e.UID == uid && e.AccountID == accountID {
				if !e.IsRead {
					m.folderInbox.DecrementUnreadCount(folderName)
					decremented = true
				}
				break
			}
		}
	}

	if decremented {
		config.SaveAccountFolders(accountID, m.folderInbox.GetFolders(), m.folderInbox.GetUnreadCountsCopy()) //nolint:errcheck,gosec
	}
}

func (m *Manager) SetFolderInbox(folderInbox FolderInboxView) {
	m.folderInbox = folderInbox
}

func (m *Manager) UpdateDependencies(deps Dependencies) {
	m.deps = deps
}

func (m *Manager) UpdateStore(store *emailstore.Store) {
	m.store = store
}

func (m *Manager) ClearPendingAction() {
	m.pendingAction = nil
	m.ActionNotice = ""
}
