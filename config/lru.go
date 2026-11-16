package config

import (
	"container/list"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Node struct {
	Key    string // key = folder:uid:accountID
	Folder string
	Body   *CachedEmailBody
}

// LRU is the in-memory index of the on-disk email body cache.
//
// Lookups (Get) are served purely from memory because they run on the UI
// goroutine. Disk writes happen outside mu, under the per-file lock of the
// folder's cache file; code holding a file lock may take mu briefly, but mu
// is never held while acquiring a file lock.
type LRU struct {
	threshold   int
	currentSize int
	cache       map[string]*list.Element
	ll          *list.List
	mu          sync.Mutex
	// dirty marks folders whose access times changed in memory since their
	// cache file was last written (see flushAccessTimes).
	dirty map[string]bool
}

type bodyRef struct {
	uid       uint32
	accountID string
}

var (
	lruMu sync.Mutex
	lru   *LRU
)

func GetLRUInstance(threshold int) *LRU {
	lruMu.Lock()
	if lru == nil {
		l := &LRU{
			threshold: threshold,
			cache:     make(map[string]*list.Element),
			ll:        list.New(),
			dirty:     make(map[string]bool),
		}
		if err := l.LoadFromDisk(); err != nil {
			log.Printf("Failed to load LRU from disk: %v\n", err)
		}
		lru = l
	}
	l := lru
	lruMu.Unlock()

	l.mu.Lock()
	var evicted []*Node
	if l.threshold != threshold {
		l.threshold = threshold
		evicted = l.evictLocked()
	}
	l.mu.Unlock()
	l.removeNodesFromDisk(evicted)

	return l
}

func (lru *LRU) makeKey(folder string, uid uint32, accountID string) string {
	return fmt.Sprintf("%s:%d:%s", folder, uid, accountID)
}

// evictLocked drops least recently used entries until the cache fits the
// threshold and returns them so the caller can delete them from disk after
// releasing mu.
func (lru *LRU) evictLocked() []*Node {
	var evicted []*Node
	for lru.currentSize > lru.threshold {
		back := lru.ll.Back()
		if back == nil {
			break
		}
		node := back.Value.(*Node)
		lru.ll.Remove(back)
		delete(lru.cache, node.Key)
		lru.currentSize -= node.Body.SizeBytes
		evicted = append(evicted, node)
	}
	return evicted
}

// removeNodesFromDisk deletes the given entries from their folders' files.
func (lru *LRU) removeNodesFromDisk(nodes []*Node) {
	byFolder := make(map[string]map[bodyRef]bool)
	for _, n := range nodes {
		if byFolder[n.Folder] == nil {
			byFolder[n.Folder] = make(map[bodyRef]bool)
		}
		byFolder[n.Folder][bodyRef{n.Body.UID, n.Body.AccountID}] = true
	}
	for folder, refs := range byFolder {
		lru.deleteFromDisk(folder, refs)
	}
}

func (lru *LRU) deleteFromDisk(folder string, refs map[bodyRef]bool) {
	_ = updateBodyCacheFile(folder, func(c *EmailBodyCache) bool {
		kept := c.Bodies[:0]
		for _, b := range c.Bodies {
			if !refs[bodyRef{b.UID, b.AccountID}] {
				kept = append(kept, b)
			}
		}
		changed := len(kept) != len(c.Bodies)
		c.Bodies = kept
		return changed
	})
}

func (lru *LRU) LoadFromDisk() error {
	dir, err := bodyCacheDir()

	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var caches []EmailBodyCache

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || filepath.Ext(name) != ".json" {
			continue
		}

		path := filepath.Join(dir, name)
		data, err := SecureReadFile(path)
		if err != nil {
			continue
		}

		var cache EmailBodyCache
		if err := json.Unmarshal(data, &cache); err != nil {
			continue
		}

		for i := range cache.Bodies {
			if cache.Bodies[i].SizeBytes <= 0 {
				cache.Bodies[i].SizeBytes = calculateEmailBodySize(&cache.Bodies[i])
			}
		}

		caches = append(caches, cache)
	}

	type bodyWithFolder struct {
		folder string
		body   CachedEmailBody
	}

	var allBodies []bodyWithFolder

	for _, cache := range caches {
		for _, body := range cache.Bodies {
			allBodies = append(allBodies, bodyWithFolder{
				folder: cache.FolderName,
				body:   body,
			})
		}
	}

	sort.Slice(allBodies, func(i, j int) bool {
		ti := allBodies[i].body.LastAccessedAt
		tj := allBodies[j].body.LastAccessedAt
		return ti.After(tj)
	})

	for i := len(allBodies) - 1; i >= 0; i-- {
		item := allBodies[i]

		if item.body.SizeBytes > lru.threshold {
			continue
		}

		key := lru.makeKey(item.folder, item.body.UID, item.body.AccountID)
		if _, dup := lru.cache[key]; dup {
			continue
		}

		bodyCopy := item.body
		node := &Node{
			Key:    key,
			Folder: item.folder,
			Body:   &bodyCopy,
		}

		e := lru.ll.PushFront(node)
		lru.cache[key] = e
		lru.currentSize += item.body.SizeBytes
	}

	if lru.currentSize > lru.threshold {
		lru.removeNodesFromDisk(lru.evictLocked())
	}
	return nil
}

// Get returns a copy of the cached body, or nil. It only touches memory: the
// access time is recorded for LRU ordering and persisted lazily.
func (lru *LRU) Get(folder string, uid uint32, accountID string) *CachedEmailBody {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	key := lru.makeKey(folder, uid, accountID)

	e, ok := lru.cache[key]

	if !ok {
		return nil
	}

	lru.ll.MoveToFront(e)

	node := e.Value.(*Node)
	node.Body.LastAccessedAt = time.Now()
	lru.dirty[folder] = true

	bodyCopy := *node.Body

	return &bodyCopy
}

func (lru *LRU) removeKey(key string) {
	if e, ok := lru.cache[key]; ok {
		node := e.Value.(*Node)

		lru.currentSize -= node.Body.SizeBytes
		lru.ll.Remove(e)
		delete(lru.cache, key)
	}
}

// applyAccessTimes copies in-memory access times into a folder file
// that is about to be written. The caller holds the file's lock.
func (lru *LRU) applyAccessTimes(folder string, c *EmailBodyCache) bool {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	changed := false
	for i := range c.Bodies {
		e, ok := lru.cache[lru.makeKey(folder, c.Bodies[i].UID, c.Bodies[i].AccountID)]
		if !ok {
			continue
		}
		if t := e.Value.(*Node).Body.LastAccessedAt; !t.Equal(c.Bodies[i].LastAccessedAt) {
			c.Bodies[i].LastAccessedAt = t
			changed = true
		}
	}
	delete(lru.dirty, folder)
	return changed
}

func (lru *LRU) Put(folder string, uid uint32, accountID string, body *CachedEmailBody) {
	key := lru.makeKey(folder, uid, accountID)
	ref := bodyRef{uid, accountID}

	lru.mu.Lock()
	// Don't cache empty bodies to prevent storing incomplete fetches, nor
	// bodies larger than the whole cache.
	if strings.TrimSpace(body.Body) == "" || body.SizeBytes > lru.threshold {
		lru.removeKey(key)
		lru.mu.Unlock()
		lru.deleteFromDisk(folder, map[bodyRef]bool{ref: true})
		return
	}

	body.LastAccessedAt = time.Now()

	if e, ok := lru.cache[key]; ok {
		node := e.Value.(*Node)
		lru.currentSize -= node.Body.SizeBytes
		lru.currentSize += body.SizeBytes
		node.Body = body
		lru.ll.MoveToFront(e)
	} else {
		node := &Node{
			Key:    key,
			Folder: folder,
			Body:   body,
		}
		e := lru.ll.PushFront(node)
		lru.cache[key] = e
		lru.currentSize += body.SizeBytes
	}

	evicted := lru.evictLocked()
	stored := *body
	lru.mu.Unlock()

	// Disk I/O happens outside mu so cache hits never wait for it.
	_ = updateBodyCacheFile(folder, func(c *EmailBodyCache) bool {
		lru.mu.Lock()
		_, stillCached := lru.cache[key]
		lru.mu.Unlock()

		idx := -1
		for i, b := range c.Bodies {
			if b.UID == uid && b.AccountID == accountID {
				idx = i
				break
			}
		}
		switch {
		case !stillCached && idx >= 0: // evicted meanwhile
			c.Bodies = append(c.Bodies[:idx], c.Bodies[idx+1:]...)
		case !stillCached:
			return false
		case idx >= 0:
			c.Bodies[idx] = stored
		default:
			c.Bodies = append(c.Bodies, stored)
		}
		lru.applyAccessTimes(folder, c)
		return true
	})
	lru.removeNodesFromDisk(evicted)
}

func (lru *LRU) Delete(folder string, uid uint32, accountID string) {
	lru.mu.Lock()
	lru.removeKey(lru.makeKey(folder, uid, accountID))
	lru.mu.Unlock()
	lru.deleteFromDisk(folder, map[bodyRef]bool{{uid, accountID}: true})
}

// pruneFolder drops every body of folder for which keep returns false, from
// memory and from disk.
func (lru *LRU) pruneFolder(folder string, keep func(uid uint32, accountID string) bool) error {
	lru.mu.Lock()
	for key, e := range lru.cache {
		node := e.Value.(*Node)
		if node.Folder == folder && !keep(node.Body.UID, node.Body.AccountID) {
			lru.removeKey(key)
		}
	}
	lru.mu.Unlock()

	return updateBodyCacheFile(folder, func(c *EmailBodyCache) bool {
		kept := c.Bodies[:0]
		for _, b := range c.Bodies {
			if keep(b.UID, b.AccountID) {
				kept = append(kept, b)
			}
		}
		changed := len(kept) != len(c.Bodies)
		c.Bodies = kept
		return changed
	})
}

// removeAccount drops every cached body of an account from memory.
func (lru *LRU) removeAccount(accountID string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	for key, e := range lru.cache {
		if e.Value.(*Node).Body.AccountID == accountID {
			lru.removeKey(key)
		}
	}
}

// flushAccessTimes writes the in-memory access times of dirty folders.
func (lru *LRU) flushAccessTimes() {
	lru.mu.Lock()
	folders := make([]string, 0, len(lru.dirty))
	for f := range lru.dirty {
		folders = append(folders, f)
	}
	lru.mu.Unlock()

	for _, folder := range folders {
		_ = updateBodyCacheFile(folder, func(c *EmailBodyCache) bool {
			return lru.applyAccessTimes(folder, c)
		})
	}
}

func resetLRU() {
	lruMu.Lock()
	lru = nil
	lruMu.Unlock()
}
