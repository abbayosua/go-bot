package bus

import "sync"

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Bus fans events out to whichever HTTP streams are watching a thread.
type Bus struct {
	mu     sync.Mutex
	subs   map[string]map[int]chan Event
	nextID int
}

func New() *Bus {
	return &Bus{subs: map[string]map[int]chan Event{}}
}

func (b *Bus) Subscribe(topic string) (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[topic] == nil {
		b.subs[topic] = map[int]chan Event{}
	}
	id := b.nextID
	b.nextID++
	ch := make(chan Event, 64)
	b.subs[topic][id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if chans, ok := b.subs[topic]; ok {
			delete(chans, id)
			if len(chans) == 0 {
				delete(b.subs, topic)
			}
		}
	}
}

// Publish never blocks a run: a slow reader loses events and resyncs from the
// message history when it reconnects.
func (b *Bus) Publish(topic string, event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs[topic] {
		select {
		case ch <- event:
		default:
		}
	}
}
