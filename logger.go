package main

import (
	"fmt"
	"log"
)

func (r *Raft) logPut(msg string) {
	logPrefix := fmt.Sprintf("[Node: %d Term: %d] ", r.me, r.currentTerm)
	log.Println(logPrefix + msg)
}
