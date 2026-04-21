package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// OfflineAction represents a queued email operation to replay when back online.
type OfflineAction struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // "mark_read", "delete", "archive", "move", "send"
	AccountID  string    `json:"account_id"`
	Folder     string    `json:"folder"`
	UIDs       []uint32  `json:"uids"`
	DestFolder string    `json:"dest_folder,omitempty"` // move only
	DraftID    string    `json:"draft_id,omitempty"`    // send only
	CreatedAt  time.Time `json:"created_at"`
}

// OfflineQueue stores all pending offline actions.
type OfflineQueue struct {
	Actions   []OfflineAction `json:"actions"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func offlineQueueFile() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "offline_queue.json"), nil
}

// SaveOfflineQueue saves the queue to disk (encrypted in secure mode).
func SaveOfflineQueue(queue *OfflineQueue) error {
	path, err := offlineQueueFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	queue.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(queue, "", "  ")
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// LoadOfflineQueue loads the queue from disk.
func LoadOfflineQueue() (*OfflineQueue, error) {
	path, err := offlineQueueFile()
	if err != nil {
		return nil, err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &OfflineQueue{}, nil
		}
		return nil, err
	}
	var queue OfflineQueue
	if err := json.Unmarshal(data, &queue); err != nil {
		return &OfflineQueue{}, nil
	}
	return &queue, nil
}

// EnqueueOfflineAction appends an action to the persistent queue.
func EnqueueOfflineAction(action OfflineAction) error {
	queue, err := LoadOfflineQueue()
	if err != nil {
		queue = &OfflineQueue{}
	}
	action.CreatedAt = time.Now()
	queue.Actions = append(queue.Actions, action)
	return SaveOfflineQueue(queue)
}

// DequeueOfflineAction removes an action by ID from the queue.
func DequeueOfflineAction(id string) error {
	queue, err := LoadOfflineQueue()
	if err != nil {
		return err
	}
	filtered := queue.Actions[:0]
	for _, a := range queue.Actions {
		if a.ID != id {
			filtered = append(filtered, a)
		}
	}
	queue.Actions = filtered
	return SaveOfflineQueue(queue)
}

// GetPendingActions returns all queued actions.
func GetPendingActions() []OfflineAction {
	queue, err := LoadOfflineQueue()
	if err != nil {
		return nil
	}
	return queue.Actions
}

// PendingActionCount returns the number of queued actions.
func PendingActionCount() int {
	queue, err := LoadOfflineQueue()
	if err != nil {
		return 0
	}
	return len(queue.Actions)
}

// ClearOfflineQueue removes all queued actions.
func ClearOfflineQueue() error {
	path, err := offlineQueueFile()
	if err != nil {
		return err
	}
	return os.Remove(path)
}
