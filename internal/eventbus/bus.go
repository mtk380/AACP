package eventbus

import "aacp/internal/types"

type Bus struct {
	events []types.Event
}

func New() *Bus {
	return &Bus{events: make([]types.Event, 0)}
}

func (b *Bus) Emit(ev types.Event) {
	b.events = append(b.events, ev)
}

func (b *Bus) Flush() []types.Event {
	out := make([]types.Event, len(b.events))
	copy(out, b.events)
	b.events = b.events[:0]
	return out
}
