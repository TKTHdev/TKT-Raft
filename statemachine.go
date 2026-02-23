package raft

import "sync"

// StateMachine is the interface users implement to plug in custom state.
// Apply is called after a log entry is committed (write path).
// Query is called after quorum confirmation without touching the log (read path).
type StateMachine interface {
	Apply(cmd []byte) []byte
	Query(cmd []byte) []byte
}

// KVStore is the built-in in-memory key-value state machine (SET/GET/DELETE).
type KVStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewKVStore() *KVStore {
	return &KVStore{data: make(map[string]string)}
}

func (kv *KVStore) Apply(cmd []byte) []byte {
	parts := splitCommand(string(cmd))
	if len(parts) == 0 {
		return nil
	}
	switch parts[0] {
	case "SET":
		if len(parts) != 3 {
			return nil
		}
		kv.mu.Lock()
		kv.data[parts[1]] = parts[2]
		kv.mu.Unlock()
	case "DELETE":
		if len(parts) != 2 {
			return nil
		}
		kv.mu.Lock()
		delete(kv.data, parts[1])
		kv.mu.Unlock()
	}
	return nil
}

func (kv *KVStore) Query(cmd []byte) []byte {
	parts := splitCommand(string(cmd))
	if len(parts) != 2 || parts[0] != "GET" {
		return nil
	}
	kv.mu.RLock()
	val := kv.data[parts[1]]
	kv.mu.RUnlock()
	return []byte(val)
}

func (r *Raft) applyCommand(command []byte, index int) {
	result := r.sm.Apply(command)

	resp := Response{
		success: true,
		value:   result,
	}

	r.mu.Lock()
	if r.state == LEADER {
		ch, ok := r.pendingResponses[index]
		if ok {
			delete(r.pendingResponses, index)
		}
		r.mu.Unlock()
		if ok {
			select {
			case ch <- resp:
			default:
			}
		}
	} else {
		r.mu.Unlock()
	}
	r.logPut("State Machine after applying command", GREEN)
}

func splitCommand(command string) []string {
	var parts []string
	current := ""
	for i := 0; i < len(command); i++ {
		if command[i] == ' ' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(command[i])
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
