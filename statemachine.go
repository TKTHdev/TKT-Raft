package main

import (
	"github.com/TKTHdev/tsujido"
)

func (r *Raft) applyCommand(command []byte, index int) {
	commandStr := string(command)
	parts := splitCommand(commandStr)
	if len(parts) == 0 {
		return
	}

	var op tsujido.Operation
	switch parts[0] {
	case "SET":
		if len(parts) != 3 {
			return
		}
		op = tsujido.Operation{Type: tsujido.OpSet, Key: parts[1], Value: parts[2]}
	case "GET":
		if len(parts) != 2 {
			return
		}
		op = tsujido.Operation{Type: tsujido.OpGet, Key: parts[1]}
	case "DELETE":
		if len(parts) != 2 {
			return
		}
		op = tsujido.Operation{Type: tsujido.OpDelete, Key: parts[1]}
	default:
		return
	}

	r.mu.Lock()
	result := r.sm.Apply(op)
	resp := Response{success: result.Success, value: result.Value}

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
