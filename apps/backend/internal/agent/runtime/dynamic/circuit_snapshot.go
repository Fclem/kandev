package dynamic

import (
	"sort"
	"strings"
)

func (r *CircuitRegistry) Get(key string) (CircuitSnapshot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, exists := r.circuits[key]
	if !exists {
		return CircuitSnapshot{}, false
	}
	return circuitSnapshot(key, entry), true
}

func (r *CircuitRegistry) List(prefix string) []CircuitSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	var snapshots []CircuitSnapshot
	for key, entry := range r.circuits {
		if strings.HasPrefix(key, prefix) {
			snapshots = append(snapshots, circuitSnapshot(key, entry))
		}
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].Key < snapshots[j].Key })
	return snapshots
}

func (r *CircuitRegistry) Close(key string) {
	if key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushPendingLocked(key)
	r.circuits[key] = circuit{state: CircuitClosed}
	_ = r.persistSnapshotLocked(key)
}

func circuitSnapshot(key string, entry circuit) CircuitSnapshot {
	return CircuitSnapshot{Key: key, State: entry.state, Until: entry.until, Code: entry.code, ProbeUntil: entry.probeUntil, ResetKnown: entry.resetKnown}
}
