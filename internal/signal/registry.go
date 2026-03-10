package signal

import (
	"sort"
	"sync"
	"time"
)

type signalDefinition struct {
	name   string
	order  int
	detect func(entries []Entry, ttl time.Duration) SignalResult
}

var (
	signalRegistryMu sync.Mutex
	signalRegistry   []signalDefinition
)

func registerSignal(definition signalDefinition) {
	signalRegistryMu.Lock()
	defer signalRegistryMu.Unlock()

	signalRegistry = append(signalRegistry, definition)
	sort.Slice(signalRegistry, func(i, j int) bool {
		if signalRegistry[i].order == signalRegistry[j].order {
			return signalRegistry[i].name < signalRegistry[j].name
		}
		return signalRegistry[i].order < signalRegistry[j].order
	})
}

func registeredSignals() []signalDefinition {
	signalRegistryMu.Lock()
	defer signalRegistryMu.Unlock()

	out := make([]signalDefinition, len(signalRegistry))
	copy(out, signalRegistry)
	return out
}
