package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sniping_engine/internal/model"
)

type CaptchaPoolItemView struct {
	ID          string `json:"id"`
	CreatedAtMs int64  `json:"createdAtMs"`
	ExpiresAtMs int64  `json:"expiresAtMs"`
	Preview     string `json:"preview,omitempty"`
}

type CaptchaPoolStatus struct {
	NowMs        int64                   `json:"nowMs"`
	Activated    bool                    `json:"activated"`
	ActivateAtMs int64                   `json:"activateAtMs"`
	DesiredSize  int                     `json:"desiredSize"`
	Size         int                     `json:"size"`
	Settings     model.CaptchaPoolSettings `json:"settings"`
	Items        []CaptchaPoolItemView   `json:"items"`
}

type captchaPoolItem struct {
	ID          string
	VerifyParam string
	CreatedAtMs int64
	ExpiresAtMs int64
}

type CaptchaPool struct {
	mu    sync.Mutex
	items []captchaPoolItem
	ch    chan struct{}

	nextID atomic.Uint64

	settings atomic.Value // model.CaptchaPoolSettings
}

func DefaultCaptchaPoolSettings() model.CaptchaPoolSettings {
	return model.CaptchaPoolSettings{
		WarmupSeconds:  30,
		PoolSize:       2,
		ItemTTLSeconds: 120,
	}
}

func normalizeCaptchaPoolSettings(in model.CaptchaPoolSettings) model.CaptchaPoolSettings {
	out := in
	if out.WarmupSeconds <= 0 {
		out.WarmupSeconds = 30
	}
	if out.PoolSize <= 0 {
		out.PoolSize = 2
	}
	if out.ItemTTLSeconds <= 0 {
		out.ItemTTLSeconds = 120
	}
	if out.PoolSize > 200 {
		out.PoolSize = 200
	}
	if out.ItemTTLSeconds > 3600 {
		out.ItemTTLSeconds = 3600
	}
	if out.WarmupSeconds > 3600 {
		out.WarmupSeconds = 3600
	}
	return out
}

func NewCaptchaPool(settings model.CaptchaPoolSettings) *CaptchaPool {
	p := &CaptchaPool{
		ch: make(chan struct{}),
	}
	p.settings.Store(normalizeCaptchaPoolSettings(settings))
	return p
}

func (p *CaptchaPool) Settings() model.CaptchaPoolSettings {
	v := p.settings.Load()
	if v == nil {
		return DefaultCaptchaPoolSettings()
	}
	if s, ok := v.(model.CaptchaPoolSettings); ok {
		return normalizeCaptchaPoolSettings(s)
	}
	return DefaultCaptchaPoolSettings()
}

func (p *CaptchaPool) SetSettings(next model.CaptchaPoolSettings) model.CaptchaPoolSettings {
	next = normalizeCaptchaPoolSettings(next)
	p.settings.Store(next)
	p.signalChanged()
	return next
}

func (p *CaptchaPool) signalChanged() {
	p.mu.Lock()
	ch := p.ch
	p.ch = make(chan struct{})
	p.mu.Unlock()
	closeChanSafe(ch)
}

func (p *CaptchaPool) pruneLocked(nowMs int64) {
	if len(p.items) == 0 {
		return
	}
	n := 0
	for _, it := range p.items {
		if it.ExpiresAtMs > 0 && it.ExpiresAtMs <= nowMs {
			continue
		}
		p.items[n] = it
		n++
	}
	if n == len(p.items) {
		return
	}
	p.items = p.items[:n]
}

func (p *CaptchaPool) Size(nowMs int64) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneLocked(nowMs)
	return len(p.items)
}

func (p *CaptchaPool) Add(verifyParam string, createdAtMs int64) (captchaPoolItem, bool) {
	verifyParam = strings.TrimSpace(verifyParam)
	if verifyParam == "" {
		return captchaPoolItem{}, false
	}
	if createdAtMs <= 0 {
		createdAtMs = time.Now().UnixMilli()
	}
	st := p.Settings()
	item := captchaPoolItem{
		ID:          fmt.Sprintf("%d-%d", createdAtMs, p.nextID.Add(1)),
		VerifyParam: verifyParam,
		CreatedAtMs: createdAtMs,
		ExpiresAtMs: createdAtMs + int64(st.ItemTTLSeconds)*1000,
	}
	p.mu.Lock()
	p.pruneLocked(time.Now().UnixMilli())
	p.items = append(p.items, item)
	p.mu.Unlock()
	p.signalChanged()
	return item, true
}

func (p *CaptchaPool) Acquire(ctx context.Context) (captchaPoolItem, bool) {
	for {
		nowMs := time.Now().UnixMilli()
		p.mu.Lock()
		p.pruneLocked(nowMs)
		if len(p.items) > 0 {
			it := p.items[0]
			copy(p.items[0:], p.items[1:])
			p.items = p.items[:len(p.items)-1]
			p.mu.Unlock()
			p.signalChanged()
			return it, true
		}
		ch := p.ch
		p.mu.Unlock()

		select {
		case <-ch:
			continue
		case <-ctx.Done():
			return captchaPoolItem{}, false
		}
	}
}

func (p *CaptchaPool) Snapshot(nowMs int64) []CaptchaPoolItemView {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneLocked(nowMs)
	out := make([]CaptchaPoolItemView, 0, len(p.items))
	for _, it := range p.items {
		out = append(out, CaptchaPoolItemView{
			ID:          it.ID,
			CreatedAtMs: it.CreatedAtMs,
			ExpiresAtMs: it.ExpiresAtMs,
			Preview:     previewVerifyParam(it.VerifyParam),
		})
	}
	return out
}

func previewVerifyParam(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	sum := sha1.Sum([]byte(v))
	return hex.EncodeToString(sum[:])[:10]
}

func closeChanSafe(ch chan struct{}) {
	defer func() { _ = recover() }()
	close(ch)
}
