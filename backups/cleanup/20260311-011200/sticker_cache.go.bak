package telegram

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// stickerCacheMu 保护 sticker cache 文件操作的并发安全。
// TS 单线程无此需求；Go 多 goroutine 并发需要锁保护避免写丢失。
var stickerCacheMu sync.Mutex

// Telegram 贴纸缓存 — 继承自 src/telegram/sticker-cache.ts (265L)

const stickerCacheVersion = 1

// StickerDescriptionPrompt 贴纸图片描述的专用 prompt。
// 对齐 TS: sticker-cache.ts STICKER_DESCRIPTION_PROMPT
const StickerDescriptionPrompt = "Describe this sticker image in 1-2 sentences. Focus on what the sticker depicts (character, object, action, emotion). Be concise and objective."

// StickerDescriptionMaxTokens 贴纸描述的最大 token 数。
const StickerDescriptionMaxTokens = 150

// StickerDescriptionTimeoutMs 贴纸描述的超时时间（毫秒）。
const StickerDescriptionTimeoutMs = 30000

// CachedSticker 缓存的贴纸数据
type CachedSticker struct {
	FileID       string `json:"fileId"`
	FileUniqueID string `json:"fileUniqueId"`
	Emoji        string `json:"emoji,omitempty"`
	SetName      string `json:"setName,omitempty"`
	Description  string `json:"description"`
	CachedAt     string `json:"cachedAt"`
	ReceivedFrom string `json:"receivedFrom,omitempty"`
}

type stickerCacheStore struct {
	Version  int                       `json:"version"`
	Stickers map[string]*CachedSticker `json:"stickers"`
}

func stickerCachePath() string {
	return filepath.Join(resolveStateDir(), "telegram", "sticker-cache.json")
}

func loadStickerCache() *stickerCacheStore {
	data, err := os.ReadFile(stickerCachePath())
	if err != nil {
		return &stickerCacheStore{Version: stickerCacheVersion, Stickers: map[string]*CachedSticker{}}
	}
	var cache stickerCacheStore
	if err := json.Unmarshal(data, &cache); err != nil || cache.Version != stickerCacheVersion {
		return &stickerCacheStore{Version: stickerCacheVersion, Stickers: map[string]*CachedSticker{}}
	}
	if cache.Stickers == nil {
		cache.Stickers = map[string]*CachedSticker{}
	}
	return &cache
}

func saveStickerCache(cache *stickerCacheStore) error {
	p := stickerCachePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(cache, "", "  ")
	return os.WriteFile(p, data, 0o600)
}

// GetCachedSticker 获取已缓存的贴纸
func GetCachedSticker(fileUniqueID string) *CachedSticker {
	stickerCacheMu.Lock()
	defer stickerCacheMu.Unlock()
	cache := loadStickerCache()
	return cache.Stickers[fileUniqueID]
}

// CacheSticker 添加/更新贴纸缓存
func CacheSticker(sticker *CachedSticker) {
	stickerCacheMu.Lock()
	defer stickerCacheMu.Unlock()
	if sticker.CachedAt == "" {
		sticker.CachedAt = time.Now().UTC().Format(time.RFC3339)
	}
	cache := loadStickerCache()
	cache.Stickers[sticker.FileUniqueID] = sticker
	_ = saveStickerCache(cache)
}

// SearchStickers 按文本查询搜索贴纸
func SearchStickers(query string, limit int) []*CachedSticker {
	stickerCacheMu.Lock()
	defer stickerCacheMu.Unlock()
	if limit <= 0 {
		limit = 10
	}
	cache := loadStickerCache()
	q := strings.ToLower(query)

	type scored struct {
		sticker *CachedSticker
		score   int
	}
	var results []scored

	for _, s := range cache.Stickers {
		score := 0
		desc := strings.ToLower(s.Description)
		if strings.Contains(desc, q) {
			score += 10
		}
		for _, w := range strings.Fields(q) {
			for _, dw := range strings.Fields(desc) {
				if strings.Contains(dw, w) {
					score += 5
				}
			}
		}
		if s.Emoji != "" && strings.Contains(query, s.Emoji) {
			score += 8
		}
		if s.SetName != "" && strings.Contains(strings.ToLower(s.SetName), q) {
			score += 3
		}
		if score > 0 {
			results = append(results, scored{s, score})
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })
	if len(results) > limit {
		results = results[:limit]
	}

	out := make([]*CachedSticker, len(results))
	for i, r := range results {
		out[i] = r.sticker
	}
	return out
}

// GetAllCachedStickers 返回所有缓存贴纸
func GetAllCachedStickers() []*CachedSticker {
	stickerCacheMu.Lock()
	defer stickerCacheMu.Unlock()
	cache := loadStickerCache()
	out := make([]*CachedSticker, 0, len(cache.Stickers))
	for _, s := range cache.Stickers {
		out = append(out, s)
	}
	return out
}

// StickerCacheStats 缓存统计
type StickerCacheStats struct {
	Count    int
	OldestAt string
	NewestAt string
}

// GetStickerCacheStats 获取缓存统计信息
func GetStickerCacheStats() StickerCacheStats {
	stickerCacheMu.Lock()
	defer stickerCacheMu.Unlock()
	cache := loadStickerCache()
	stickers := make([]*CachedSticker, 0, len(cache.Stickers))
	for _, s := range cache.Stickers {
		stickers = append(stickers, s)
	}
	if len(stickers) == 0 {
		return StickerCacheStats{}
	}
	sort.Slice(stickers, func(i, j int) bool { return stickers[i].CachedAt < stickers[j].CachedAt })
	return StickerCacheStats{
		Count:    len(stickers),
		OldestAt: stickers[0].CachedAt,
		NewestAt: stickers[len(stickers)-1].CachedAt,
	}
}

// DescribeStickerImage 描述贴纸图片内容。
// 先检查缓存，缓存未命中时通过 DI 回调获取描述，然后缓存结果。
// describer 为 nil 时返回 emoji+setName 作为 fallback。
func DescribeStickerImage(sticker *TelegramSticker, describer func(fileID string) (string, error)) string {
	if sticker == nil {
		return ""
	}

	// 检查缓存
	cached := GetCachedSticker(sticker.FileUniqueID)
	if cached != nil && cached.Description != "" {
		return cached.Description
	}

	// fallback: emoji + setName
	fallback := ""
	if sticker.Emoji != "" {
		fallback = sticker.Emoji
	}
	if sticker.SetName != "" {
		if fallback != "" {
			fallback += " "
		}
		fallback += "from sticker set: " + sticker.SetName
	}
	if fallback == "" {
		fallback = "sticker"
	}

	// 如果没有 DI 描述器，返回 fallback
	if describer == nil {
		return fallback
	}

	// 调用 DI 描述器
	desc, err := describer(sticker.FileID)
	if err != nil || desc == "" {
		return fallback
	}

	// 缓存结果
	CacheSticker(&CachedSticker{
		FileID:       sticker.FileID,
		FileUniqueID: sticker.FileUniqueID,
		Emoji:        sticker.Emoji,
		SetName:      sticker.SetName,
		Description:  desc,
	})

	return desc
}
