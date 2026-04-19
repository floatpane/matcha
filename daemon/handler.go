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

func (d *Daemon) handleRequest(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	switch req.Method {
	case daemonrpc.MethodPing:
		d.handlePing(conn, req)
	case daemonrpc.MethodGetStatus:
		d.handleGetStatus(conn, req)
	case daemonrpc.MethodGetAccounts:
		d.handleGetAccounts(conn, req)
	case daemonrpc.MethodReloadConfig:
		d.handleReloadConfig(conn, req)
	case daemonrpc.MethodFetchEmails:
		d.handleFetchEmails(conn, req)
	case daemonrpc.MethodFetchEmailBody:
		d.handleFetchEmailBody(conn, req)
	case daemonrpc.MethodDeleteEmails:
		d.handleDeleteEmails(conn, req)
	case daemonrpc.MethodArchiveEmails:
		d.handleArchiveEmails(conn, req)
	case daemonrpc.MethodMoveEmails:
		d.handleMoveEmails(conn, req)
	case daemonrpc.MethodMarkRead:
		d.handleMarkRead(conn, req)
	case daemonrpc.MethodFetchFolders:
		d.handleFetchFolders(conn, req)
	case daemonrpc.MethodRefreshFolder:
		d.handleRefreshFolder(conn, req)
	case daemonrpc.MethodSubscribe:
		d.handleSubscribe(conn, req)
	case daemonrpc.MethodUnsubscribe:
		d.handleUnsubscribe(conn, req)
	default:
		conn.SendError(req.ID, daemonrpc.ErrCodeNotFound, fmt.Sprintf("unknown method: %s", req.Method))
	}
}

func decodeParams[T any](req *daemonrpc.Request) (T, error) {
	var params T
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return params, err
		}
	}
	return params, nil
}

func (d *Daemon) handlePing(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	conn.SendResponse(req.ID, daemonrpc.PingResult{Pong: true})
}

func (d *Daemon) handleGetStatus(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	d.mu.RLock()
	var accounts []string
	for _, acct := range d.config.Accounts {
		accounts = append(accounts, acct.Email)
	}
	d.mu.RUnlock()

	conn.SendResponse(req.ID, daemonrpc.StatusResult{
		Running:  true,
		Uptime:   int64(time.Since(d.startTime).Seconds()),
		Accounts: accounts,
		PID:      os.Getpid(),
	})
}

func (d *Daemon) handleGetAccounts(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var infos []daemonrpc.AccountInfo
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
	conn.SendResponse(req.ID, infos)
}

func (d *Daemon) handleReloadConfig(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	if err := d.ReloadConfig(); err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}
	conn.SendResponse(req.ID, true)
}

func (d *Daemon) handleFetchEmails(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.FetchEmailsParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	p, err := d.getProvider(params.AccountID)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	emails, err := p.FetchEmails(ctx, params.Folder, params.Limit, params.Offset)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}

	conn.SendResponse(req.ID, emails)
}

func (d *Daemon) handleFetchEmailBody(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.FetchEmailBodyParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	p, err := d.getProvider(params.AccountID)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	body, attachments, err := p.FetchEmailBody(ctx, params.Folder, params.UID)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
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

	conn.SendResponse(req.ID, daemonrpc.FetchEmailBodyResult{
		Body:        body,
		Attachments: attInfos,
	})
}

func (d *Daemon) handleDeleteEmails(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.DeleteEmailsParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	p, err := d.getProvider(params.AccountID)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.DeleteEmails(ctx, params.Folder, params.UIDs); err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}
	conn.SendResponse(req.ID, true)
}

func (d *Daemon) handleArchiveEmails(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.ArchiveEmailsParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	p, err := d.getProvider(params.AccountID)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.ArchiveEmails(ctx, params.Folder, params.UIDs); err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}
	conn.SendResponse(req.ID, true)
}

func (d *Daemon) handleMoveEmails(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.MoveEmailsParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	p, err := d.getProvider(params.AccountID)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.MoveEmails(ctx, params.UIDs, params.SourceFolder, params.DestFolder); err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}
	conn.SendResponse(req.ID, true)
}

func (d *Daemon) handleMarkRead(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.MarkReadParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	p, err := d.getProvider(params.AccountID)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// MarkAsRead only supports one UID at a time in the Provider interface.
	for _, uid := range params.UIDs {
		if err := p.MarkAsRead(ctx, params.Folder, uid); err != nil {
			log.Printf("daemon: mark read %d failed: %v", uid, err)
		}
	}
	conn.SendResponse(req.ID, true)
}

func (d *Daemon) handleFetchFolders(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.FetchFoldersParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	p, err := d.getProvider(params.AccountID)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	folders, err := p.FetchFolders(ctx)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeInternal, err.Error())
		return
	}
	conn.SendResponse(req.ID, folders)
}

func (d *Daemon) handleRefreshFolder(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.RefreshFolderParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	// Async: fetch in background, push events when done.
	go func() {
		p, err := d.getProvider(params.AccountID)
		if err != nil {
			log.Printf("daemon: refresh provider error: %v", err)
			return
		}

		d.broadcastToSubscribers(params.AccountID, params.Folder, daemonrpc.EventSyncStarted, daemonrpc.SyncStartedEvent{
			AccountID: params.AccountID,
			Folder:    params.Folder,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		emails, err := p.FetchEmails(ctx, params.Folder, 50, 0)
		if err != nil {
			d.broadcastToSubscribers(params.AccountID, params.Folder, daemonrpc.EventSyncError, daemonrpc.SyncErrorEvent{
				AccountID: params.AccountID,
				Folder:    params.Folder,
				Error:     err.Error(),
			})
			return
		}

		d.broadcastToSubscribers(params.AccountID, params.Folder, daemonrpc.EventSyncComplete, daemonrpc.SyncCompleteEvent{
			AccountID:  params.AccountID,
			Folder:     params.Folder,
			EmailCount: len(emails),
		})
	}()

	conn.SendResponse(req.ID, true)
}

func (d *Daemon) handleSubscribe(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.SubscribeParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	key := params.AccountID + ":" + params.Folder

	d.subMu.Lock()
	if d.subscriptions[conn] == nil {
		d.subscriptions[conn] = make(map[string]struct{})
	}
	d.subscriptions[conn][key] = struct{}{}
	d.subMu.Unlock()

	log.Printf("daemon: client subscribed to %s", key)
	conn.SendResponse(req.ID, true)
}

func (d *Daemon) handleUnsubscribe(conn *daemonrpc.Conn, req *daemonrpc.Request) {
	params, err := decodeParams[daemonrpc.UnsubscribeParams](req)
	if err != nil {
		conn.SendError(req.ID, daemonrpc.ErrCodeParse, err.Error())
		return
	}

	key := params.AccountID + ":" + params.Folder

	d.subMu.Lock()
	if subs, ok := d.subscriptions[conn]; ok {
		delete(subs, key)
	}
	d.subMu.Unlock()

	conn.SendResponse(req.ID, true)
}
