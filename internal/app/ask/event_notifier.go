package ask

import "sync"

// RunEventNotifier 是进程内的事件广播器：事件落库后通知当前 Run 的订阅者，
// 使事件流不必依赖固定轮询间隔即可尽快下发。通知只表达「有新事件」，
// 订阅者仍按已落库的事件重读，保证顺序与持久化事实一致。
type RunEventNotifier struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

func NewRunEventNotifier() *RunEventNotifier {
	return &RunEventNotifier{subs: make(map[string]map[chan struct{}]struct{})}
}

// Subscribe 订阅某个 Run 的事件通知，返回通知通道与解除订阅函数。
func (n *RunEventNotifier) Subscribe(runID string) (<-chan struct{}, func()) {
	if n == nil {
		return make(chan struct{}), func() {}
	}
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	if n.subs[runID] == nil {
		n.subs[runID] = make(map[chan struct{}]struct{})
	}
	n.subs[runID][ch] = struct{}{}
	n.mu.Unlock()
	return ch, func() {
		n.mu.Lock()
		if set, ok := n.subs[runID]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(n.subs, runID)
			}
		}
		n.mu.Unlock()
	}
}

// Notify 通知某个 Run 的订阅者；通道已满时跳过，订阅者会读到全部已落库事件。
func (n *RunEventNotifier) Notify(runID string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.subs[runID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
