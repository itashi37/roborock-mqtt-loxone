package health

import (
	"sync"
	"time"
)

const DefaultHeartbeatInterval = 30 * time.Second

// Snapshot separates bridge process health, Roborock cloud reachability and
// per-robot availability. Heartbeat is a Unix timestamp suitable for a Loxone
// 90-second timeout.
type Snapshot struct {
	BridgeAlive    bool
	CloudConnected bool
	RobotOnline    map[string]bool
	Heartbeat      int64
}

type Source func(time.Time) Snapshot
type Sink func(Snapshot)

// Publisher emits a fresh snapshot periodically even if no state changed.
// PublishNow is safe to call after transport reconnects and configuration
// changes so consumers do not wait for the next heartbeat tick.
type Publisher struct {
	interval time.Duration
	source   Source
	sink     Sink
	now      func() time.Time

	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
	running bool
}

func NewPublisher(interval time.Duration, source Source, sink Sink) *Publisher {
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	return &Publisher{interval: interval, source: source, sink: sink, now: time.Now}
}

func (p *Publisher) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.stop = make(chan struct{})
	p.done = make(chan struct{})
	stop, done := p.stop, p.done
	p.mu.Unlock()

	p.PublishNow()
	go func() {
		defer close(done)
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.PublishNow()
			case <-stop:
				return
			}
		}
	}()
}

func (p *Publisher) PublishNow() {
	if p.source == nil || p.sink == nil {
		return
	}
	now := p.now()
	snapshot := p.source(now)
	snapshot.Heartbeat = now.Unix()
	if snapshot.RobotOnline == nil {
		snapshot.RobotOnline = map[string]bool{}
	}
	p.sink(snapshot)
}

func (p *Publisher) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	stop, done := p.stop, p.done
	p.running = false
	close(stop)
	p.mu.Unlock()
	<-done
}
