package logbus

import (
	"sync"
	"time"
)

type Message struct {
	Type string `json:"type"`
	Time int64  `json:"time"`
	Data any    `json:"data"`
}

type LogData struct {
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Fields map[string]any `json:"fields,omitempty"`
}

type Bus struct {
	mu     sync.RWMutex
	buf    []Message
	cap    int
	subs   map[chan Message]struct{}
	closed bool
}

func New(capacity int) *Bus {
	if capacity <= 0 {
		capacity = 200
	}
	return &Bus{
		cap:  capacity,
		buf:  make([]Message, 0, capacity),
		subs: make(map[chan Message]struct{}),
	}
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		close(ch)
	}
	b.subs = nil
	b.buf = nil
}

func (b *Bus) Snapshot() []Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Message, len(b.buf))
	copy(out, b.buf)
	return out
}

func (b *Bus) Subscribe(buffer int) (<-chan Message, func()) {
	if buffer <= 0 {
		buffer = 64
	}
	ch := make(chan Message, buffer)
	b.mu.Lock()
	if b.closed {
		close(ch)
		b.mu.Unlock()
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if b.subs != nil {
			if _, ok := b.subs[ch]; ok {
				delete(b.subs, ch)
				close(ch)
			}
		}
		b.mu.Unlock()
	}
	return ch, cancel
}

func (b *Bus) Publish(typ string, data any) {
	msg := Message{
		Type: typ,
		Time: time.Now().UnixMilli(),
		Data: data,
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	if len(b.buf) < b.cap {
		b.buf = append(b.buf, msg)
	} else if b.cap > 0 {
		copy(b.buf, b.buf[1:])
		b.buf[b.cap-1] = msg
	}
	for ch := range b.subs {
		select {
		case ch <- msg:
		default:
		}
	}
	b.mu.Unlock()
}

func (b *Bus) Log(level, message string, fields map[string]any) {
	b.Publish("log", LogData{Level: level, Msg: message, Fields: fields})
}

