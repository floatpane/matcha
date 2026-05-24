package config

import (
	"container/list"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Node struct {
	Key    string // key = folder:uid:accountID
	Folder string
	Body   *CachedEmailBody
}

type LRU struct {
	threshold   int
	currentSize int
	cache       map[string]*list.Element
	ll          *list.List
	mu          sync.Mutex
}

var lru *LRU
var once sync.Once

func GetLRUInstance(threshold int) *LRU {
	once.Do(
		func() {
			lru = &LRU{
				threshold: threshold,
				cache:     make(map[string]*list.Element),
				ll:        list.New(),
			}

			if err := lru.LoadFromDisk(); err != nil {
				log.Printf("Failed to load LRU from disk: %v\n", err)
			}
		})

	lru.mu.Lock()
	defer lru.mu.Unlock()

	if lru.threshold != threshold {
		lru.threshold = threshold
		if lru.currentSize > lru.threshold {
			lru.evict()
		}
	}

	return lru
}

func (lru *LRU) makeKey(folder string, uid uint32, accountID string) string {
	return fmt.Sprintf("%s:%d:%s", folder, uid, accountID)
}

func removeBodyFromDisk(folder string, uid uint32, accountID string) error {
	cache, err := LoadEmailBodyCache(folder)

	if err != nil {
		return err
	}

	kept := cache.Bodies[:0]
	for _, b := range cache.Bodies {
		if b.UID != uid || b.AccountID != accountID {
			kept = append(kept, b)
		}
	}

	if len(kept) == len(cache.Bodies) {
		return nil
	}

	cache.Bodies = kept
	return saveEmailBodyCache(cache)
}

func (lru *LRU) evict() {
	for lru.currentSize > lru.threshold {
		back := lru.ll.Back()

		if back == nil {
			break
		}

		node := back.Value.(*Node)

		lru.ll.Remove(back)
		delete(lru.cache, node.Key)
		lru.currentSize -= node.Body.SizeBytes

		_ = removeBodyFromDisk(node.Folder, node.Body.UID, node.Body.AccountID)
	}
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
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
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
		lru.evict()
	}
	return nil
}

func saveEmailBodyToDisk(folder string, body *CachedEmailBody) error {
	cache, err := LoadEmailBodyCache(folder)

	if err != nil {
		cache = &EmailBodyCache{FolderName: folder}
	}

	found := false
	for i, b := range cache.Bodies {
		if b.UID == body.UID && b.AccountID == body.AccountID {
			cache.Bodies[i] = *body
			found = true
			break
		}
	}
	if !found {
		cache.Bodies = append(cache.Bodies, *body)
	}

	return saveEmailBodyCache(cache)
}

func (lru *LRU) Get(folder string, uid uint32, accountID string) *Node {
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

	_ = saveEmailBodyToDisk(folder, node.Body)

	return node
}

func (lru *LRU) removeKey(key string) {
	if e, ok := lru.cache[key]; ok {
		node := e.Value.(*Node)

		lru.currentSize -= node.Body.SizeBytes
		lru.ll.Remove(e)
		delete(lru.cache, key)
	}
}

func (lru *LRU) Put(folder string, uid uint32, accountID string, body *CachedEmailBody) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	key := lru.makeKey(folder, uid, accountID)

	if body.SizeBytes > lru.threshold {
		lru.removeKey(key)
		_ = removeBodyFromDisk(folder, uid, accountID)
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

	lru.evict()

	_ = saveEmailBodyToDisk(folder, body)
}

func (lru *LRU) Delete(folder string, uid uint32, accountID string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	key := lru.makeKey(folder, uid, accountID)
	lru.removeKey(key)
	_ = removeBodyFromDisk(folder, uid, accountID)
}

func resetLRU() {
	once = sync.Once{}
	lru = nil
}
