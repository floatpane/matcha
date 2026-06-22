package exportcmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/export"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/loglevel"
	"github.com/floatpane/matcha/tui"
)

const (
	goosDarwin  = "darwin"
	goosLinux   = "linux"
	goosWindows = "windows"
)

// SanitizeFilename prevents path traversal attacks on attachment downloads.
// Email attachment filenames come from untrusted email headers and could
// contain path separators or ".." sequences to escape the Downloads directory.
func SanitizeFilename(name string) string {
	// Normalize backslashes to forward slashes so filepath.Base works
	// correctly on all platforms (Linux doesn't treat \ as a separator)
	name = strings.ReplaceAll(name, "\\", "/")
	// Strip any path components, keep only the base filename
	name = filepath.Base(name)
	// Replace any remaining path separators (defensive)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "..", "_")
	// Reject hidden files and empty names
	if name == "" || name == "." || strings.HasPrefix(name, ".") {
		name = "attachment"
	}
	// Sanitize filename: enforce length limit to prevent filesystem errors
	// with extremely long names from untrusted email headers.
	const maxFilenameLen = 255
	if len(name) > maxFilenameLen {
		ext := filepath.Ext(name)
		if len(ext) > maxFilenameLen {
			ext = TruncateUTF8(ext, maxFilenameLen)
		}
		base := strings.TrimSuffix(name, ext)
		name = TruncateUTF8(base, maxFilenameLen-len(ext)) + ext
	}
	return name
}

// TruncateUTF8 truncates a string to at most maxBytes bytes while keeping it valid UTF-8.
func TruncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for !utf8.ValidString(s) {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

// DownloadAttachmentCmd downloads an email attachment and saves it to the
// user's Downloads folder.
func DownloadAttachmentCmd(account *config.Account, uid uint32, msg tui.DownloadAttachmentMsg) tea.Cmd {
	return func() tea.Msg {
		var data []byte
		var err error
		switch msg.Mailbox {
		case tui.MailboxSent:
			data, err = fetcher.FetchSentAttachment(account, uid, msg.PartID, msg.Encoding)
		case tui.MailboxTrash:
			data, err = fetcher.FetchTrashAttachment(account, uid, msg.PartID, msg.Encoding)
		case tui.MailboxArchive:
			data, err = fetcher.FetchArchiveAttachment(account, uid, msg.PartID, msg.Encoding)
		case tui.MailboxInbox:
			data, err = fetcher.FetchAttachment(account, uid, msg.PartID, msg.Encoding)
		}

		if err != nil {
			return tui.AttachmentDownloadedMsg{Err: err}
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return tui.AttachmentDownloadedMsg{Err: err}
		}
		downloadsPath := filepath.Join(homeDir, "Downloads")
		if _, err := os.Stat(downloadsPath); os.IsNotExist(err) {
			if mkErr := os.MkdirAll(downloadsPath, 0750); mkErr != nil {
				return tui.AttachmentDownloadedMsg{Err: mkErr}
			}
		}

		origName := SanitizeFilename(msg.Filename)
		ext := filepath.Ext(origName)
		base := strings.TrimSuffix(origName, ext)
		candidate := origName
		i := 1
		var filePath string

		for {
			filePath = filepath.Join(downloadsPath, candidate)

			f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				if os.IsExist(err) {
					candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
					i++
					continue
				}
				log.Printf("error creating file %s: %v", filePath, err)
				return tui.AttachmentDownloadedMsg{Err: err}
			}

			if _, writeErr := f.Write(data); writeErr != nil {
				_ = f.Close()
				log.Printf("error writing to file %s: %v", filePath, writeErr)
				return tui.AttachmentDownloadedMsg{Err: writeErr}
			}
			if closeErr := f.Close(); closeErr != nil {
				log.Printf("warning: error closing file %s: %v", filePath, closeErr)
			}

			break
		}

		log.Printf("attachment saved to %s", filePath)

		go func(p string) {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case goosDarwin:
				cmd = exec.Command("open", p) //nolint:noctx
			case goosLinux:
				cmd = exec.Command("xdg-open", p) //nolint:noctx
			case goosWindows:
				cmd = exec.Command("cmd", "/c", "start", "", p) //nolint:noctx
			default:
				return
			}
			if err := cmd.Start(); err != nil {
				log.Printf("failed to open file %s: %v", p, err)
			}
		}(filePath)

		return tui.AttachmentDownloadedMsg{Path: filePath, Err: nil}
	}
}

// ExportEmailCmd fetches the raw RFC822 message, converts it to the desired
// format (HTML or Markdown) with full metadata, and writes it to savePath.
func ExportEmailCmd(cfg *config.Config, email fetcher.Email, accountID, folderName, format, savePath string) tea.Cmd {
	return func() tea.Msg {
		account := cfg.GetAccountByID(accountID)
		if account == nil {
			return tui.EmailExportedMsg{Err: fmt.Errorf("account not found")}
		}

		rawMsg, err := fetcher.FetchRawMessageFromMailbox(account, folderName, email.UID)
		if err != nil {
			return tui.EmailExportedMsg{Err: fmt.Errorf("failed to fetch raw message: %w", err)}
		}

		var data []byte
		switch format {
		case "markdown", "md":
			data, err = export.EmailToMarkdown(rawMsg, email)
		default:
			data, err = export.EmailToHTML(rawMsg, email)
		}
		if err != nil {
			return tui.EmailExportedMsg{Err: fmt.Errorf("failed to convert email: %w", err)}
		}

		if err := export.WriteToFile(savePath, data); err != nil {
			return tui.EmailExportedMsg{Err: fmt.Errorf("failed to write file: %w", err)}
		}

		loglevel.Infof("email exported to %s", savePath)
		return tui.EmailExportedMsg{Path: savePath, Err: nil}
	}
}

// OpenEmailInBrowserCmd saves the raw email HTML to a temp file and opens it
// in the system's default browser.
func OpenEmailInBrowserCmd(cfg *config.Config, email fetcher.Email, accountID, folderName string) tea.Cmd {
	return func() tea.Msg {
		account := cfg.GetAccountByID(accountID)
		if account == nil {
			return tui.EmailOpenedInBrowserMsg{Err: fmt.Errorf("account not found")}
		}

		rawMsg, err := fetcher.FetchRawMessageFromMailbox(account, folderName, email.UID)
		if err != nil {
			return tui.EmailOpenedInBrowserMsg{Err: fmt.Errorf("failed to fetch raw message: %w", err)}
		}

		htmlData, err := export.EmailToHTML(rawMsg, email)
		if err != nil {
			return tui.EmailOpenedInBrowserMsg{Err: fmt.Errorf("failed to convert email: %w", err)}
		}

		tmpFile, err := os.CreateTemp("", "matcha-email-*.html")
		if err != nil {
			return tui.EmailOpenedInBrowserMsg{Err: fmt.Errorf("failed to create temp file: %w", err)}
		}
		if _, err := tmpFile.Write(htmlData); err != nil {
			tmpFile.Close() //nolint:errcheck,gosec
			return tui.EmailOpenedInBrowserMsg{Err: fmt.Errorf("failed to write temp file: %w", err)}
		}
		if err := tmpFile.Close(); err != nil {
			return tui.EmailOpenedInBrowserMsg{Err: fmt.Errorf("failed to close temp file: %w", err)}
		}

		tmpPath := tmpFile.Name()
		loglevel.Debugf("email saved to temp file %s for browser viewing", tmpPath)

		var cmd *exec.Cmd
		switch runtime.GOOS {
		case goosDarwin:
			cmd = exec.Command("open", tmpPath) //nolint:noctx
		case goosLinux:
			cmd = exec.Command("xdg-open", tmpPath) //nolint:noctx
		case goosWindows:
			cmd = exec.Command("cmd", "/c", "start", "", tmpPath) //nolint:noctx
		default:
			return tui.EmailOpenedInBrowserMsg{Err: fmt.Errorf("unsupported OS: %s", runtime.GOOS)}
		}
		if err := cmd.Start(); err != nil {
			return tui.EmailOpenedInBrowserMsg{Err: fmt.Errorf("failed to open browser: %w", err)}
		}

		return tui.EmailOpenedInBrowserMsg{Err: nil}
	}
}
