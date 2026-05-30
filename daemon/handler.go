package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/floatpane/matcha/daemonrpc"
)

// Per-handler timeouts. fetchTimeout covers reads against the upstream IMAP
// provider, which can return large bodies and so are given more headroom.
// mutateTimeout covers state-changing operations and folder listings, which
// are bounded by IMAP command latency rather than payload size.
const (
	fetchTimeout  = 60 * time.Second
	mutateTimeout = 30 * time.Second
)

// decodeParams unmarshals raw JSON params into T. A nil/empty payload yields
// the zero value.
func decodeParams[T any](params json.RawMessage) (T, error) {
	var p T
	if params != nil {
		if err := json.Unmarshal(params, &p); err != nil {
			return p, err
		}
	}
	return p, nil
}

// parseError wraps a params-decoding failure with the parse error code so the
// server forwards it verbatim instead of mapping to ErrCodeInternal.
func parseError(err error) error {
	return &daemonrpc.Error{Code: daemonrpc.ErrCodeParse, Message: err.Error()}
}

func (d *Daemon) handlePing(_ context.Context, _ *daemonrpc.Conn, _ json.RawMessage) (any, error) {
	return daemonrpc.PingResult{Pong: true}, nil
}

func (d *Daemon) handleGetStatus(_ context.Context, _ *daemonrpc.Conn, _ json.RawMessage) (any, error) {
	d.mu.RLock()
	accounts := make([]string, 0, len(d.config.Accounts))
	for _, acct := range d.config.Accounts {
		accounts = append(accounts, acct.Email)
	}
	d.mu.RUnlock()

	return daemonrpc.StatusResult{
		Running:  true,
		Uptime:   int64(time.Since(d.startTime).Seconds()),
		Accounts: accounts,
		PID:      os.Getpid(),
	}, nil
}

func (d *Daemon) handleGetAccounts(_ context.Context, _ *daemonrpc.Conn, _ json.RawMessage) (any, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	infos := make([]daemonrpc.AccountInfo, 0, len(d.config.Accounts))
	for _, acct := range d.config.Accounts {
		protocol := acct.Protocol
		if protocol == "" {
			protocol = "imap"
		}
		infos = append(infos, daemonrpc.AccountInfo{
			ID:       acct.ID,
			Name:     acct.Name,
			Email:    acct.Email,
			Protocol: protocol,
		})
	}
	return infos, nil
}

func (d *Daemon) handleReloadConfig(_ context.Context, _ *daemonrpc.Conn, _ json.RawMessage) (any, error) {
	if err := d.ReloadConfig(); err != nil {
		return nil, err
	}
	return true, nil
}

func (d *Daemon) handleFetchEmails(ctx context.Context, _ *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.FetchEmailsParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	p, err := d.getProvider(args.AccountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	emails, err := p.FetchEmails(ctx, args.Folder, args.Limit, args.Offset)
	if err != nil {
		return nil, err
	}
	return emails, nil
}

func (d *Daemon) handleFetchEmailBody(ctx context.Context, _ *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.FetchEmailBodyParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	p, err := d.getProvider(args.AccountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	body, mimeType, attachments, err := p.FetchEmailBody(ctx, args.Folder, args.UID)
	if err != nil {
		return nil, err
	}

	// Convert backend.Attachment to daemonrpc.AttachmentInfo for wire transfer.
	var attInfos []daemonrpc.AttachmentInfo
	for _, att := range attachments {
		attInfos = append(attInfos, daemonrpc.AttachmentInfo{
			Filename: att.Filename,
			PartID:   att.PartID,
			Encoding: att.Encoding,
			MIMEType: att.MIMEType,
		})
	}

	return daemonrpc.FetchEmailBodyResult{
		Body:         body,
		BodyMIMEType: mimeType,
		Attachments:  attInfos,
	}, nil
}

func (d *Daemon) handleDeleteEmails(ctx context.Context, _ *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.DeleteEmailsParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	p, err := d.getProvider(args.AccountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, mutateTimeout)
	defer cancel()

	if err := p.DeleteEmails(ctx, args.Folder, args.UIDs); err != nil {
		return nil, err
	}
	return true, nil
}

func (d *Daemon) handleArchiveEmails(ctx context.Context, _ *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.ArchiveEmailsParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	p, err := d.getProvider(args.AccountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, mutateTimeout)
	defer cancel()

	if err := p.ArchiveEmails(ctx, args.Folder, args.UIDs); err != nil {
		return nil, err
	}
	return true, nil
}

func (d *Daemon) handleMoveEmails(ctx context.Context, _ *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.MoveEmailsParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	p, err := d.getProvider(args.AccountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, mutateTimeout)
	defer cancel()

	if err := p.MoveEmails(ctx, args.UIDs, args.SourceFolder, args.DestFolder); err != nil {
		return nil, err
	}
	return true, nil
}

func (d *Daemon) handleMarkRead(ctx context.Context, _ *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.MarkReadParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	p, err := d.getProvider(args.AccountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, mutateTimeout)
	defer cancel()

	for _, uid := range args.UIDs {
		var err error
		if args.Read {
			err = p.MarkAsRead(ctx, args.Folder, uid)
		} else {
			err = p.MarkAsUnread(ctx, args.Folder, uid)
		}
		if err != nil {
			log.Printf("daemon: mark read=%v %d failed: %v", args.Read, uid, err)
		}
	}
	return true, nil
}

func (d *Daemon) handleFetchFolders(ctx context.Context, _ *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.FetchFoldersParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	p, err := d.getProvider(args.AccountID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, mutateTimeout)
	defer cancel()

	folders, err := p.FetchFolders(ctx)
	if err != nil {
		return nil, err
	}
	return folders, nil
}

func (d *Daemon) handleRefreshFolder(ctx context.Context, _ *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.RefreshFolderParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	// Async: fetch in background, push events when done. The server-scoped ctx
	// outlives the request and is canceled on daemon shutdown.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("daemon: refresh panic for account = %s folder = %s: %v", args.AccountID, args.Folder, r)
				d.broadcastToSubscribers(args.AccountID, args.Folder, daemonrpc.EventSyncError, daemonrpc.SyncErrorEvent{
					AccountID: args.AccountID,
					Folder:    args.Folder,
					Error:     fmt.Sprintf("panic: %v", r),
				})
			}
		}()

		p, err := d.getProvider(args.AccountID)
		if err != nil {
			log.Printf("daemon: refresh provider error: %v", err)
			return
		}

		d.broadcastToSubscribers(args.AccountID, args.Folder, daemonrpc.EventSyncStarted, daemonrpc.SyncStartedEvent(args))

		fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
		defer cancel()

		emails, err := p.FetchEmails(fetchCtx, args.Folder, 50, 0)
		if err != nil {
			d.broadcastToSubscribers(args.AccountID, args.Folder, daemonrpc.EventSyncError, daemonrpc.SyncErrorEvent{
				AccountID: args.AccountID,
				Folder:    args.Folder,
				Error:     err.Error(),
			})
			return
		}

		d.broadcastToSubscribers(args.AccountID, args.Folder, daemonrpc.EventSyncComplete, daemonrpc.SyncCompleteEvent{
			AccountID:  args.AccountID,
			Folder:     args.Folder,
			EmailCount: len(emails),
		})
	}()

	return true, nil
}

func (d *Daemon) handleSubscribe(_ context.Context, conn *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.SubscribeParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	key := args.AccountID + ":" + args.Folder

	d.subMu.Lock()
	if d.subscriptions[conn] == nil {
		d.subscriptions[conn] = make(map[string]struct{})
	}
	d.subscriptions[conn][key] = struct{}{}
	d.subMu.Unlock()

	log.Printf("daemon: client subscribed to %s", key)
	return true, nil
}

func (d *Daemon) handleUnsubscribe(_ context.Context, conn *daemonrpc.Conn, params json.RawMessage) (any, error) {
	args, err := decodeParams[daemonrpc.UnsubscribeParams](params)
	if err != nil {
		return nil, parseError(err)
	}

	key := args.AccountID + ":" + args.Folder

	d.subMu.Lock()
	if subs, ok := d.subscriptions[conn]; ok {
		delete(subs, key)
	}
	d.subMu.Unlock()

	return true, nil
}
