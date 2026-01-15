package main

import (
)

func (r *Raft) applyCommand(command []byte, index int) {
	commandStr := string(command)
	parts := splitCommand(commandStr)
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "SET":
		if len(parts) != 3 {
			return
		}
		r.mu.Lock()
		key := parts[1]
		value := parts[2]
		r.StateMachine[key] = value
		r.mu.Unlock()

	case "GET":
		if len(parts) != 2 {
			return
		}
		r.mu.Lock()
		key := parts[1]
		_ = r.StateMachine[key] // In a real implementation, you might want to return this value.
		r.mu.Unlock()
	case "DELETE":
		if len(parts) != 2 {
			return
		}
		r.mu.Lock()
		key := parts[1]

		delete(r.StateMachine, key)
		r.mu.Unlock()
	default:
		// Unknown command
	}
	Response := Response{
		success: true,
	}
	
	r.mu.Lock()
	if parts[0] == "GET" && len(parts) == 2 {
		key := parts[1]
		value, exists := r.StateMachine[key]
		if exists {
			Response.value = value
		} else {
			Response.success = true
		}
	}
	if r.state == LEADER {
		ch, ok := r.pendingResponses[index]
		if ok {
			delete(r.pendingResponses, index)
		}
		r.mu.Unlock()

		if ok {
			select {
			case ch <- Response:
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
