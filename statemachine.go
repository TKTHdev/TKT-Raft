package main

import(
	"fmt"
)

func (r *Raft) applyCommand(command []byte) {
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
		key := parts[1]
		value := parts[2]
		r.StateMachine[key] = value

	case "GET":
		if len(parts) != 2 {
			return
		}
		key := parts[1]
		_ = r.StateMachine[key] // In a real implementation, you might want to return this value.
	case "DELETE":
		if len(parts) != 2 {
			return
		}
		key := parts[1]

		delete(r.StateMachine, key)
	default:
		// Unknown command
	}
	Response := Response{
		success: true,
	}
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
		select {
		case r.RespCh <- Response:
		default:
		}
	}
	r.logPut(fmt.Sprintf("State Machine after applying command: %s", r.printStateMachineAsString()), GREEN)
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
