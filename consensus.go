package main

import (
	"log"
	"math/rand"
	"time"
)

const (
	MINELECTION_TIMEOUT = 150 * time.Millisecond
	MAXELECTION_TIMEOUT = 300 * time.Millisecond
)

func (r *Raft) Run() {
	for {
		timeout := time.Duration(MINELECTION_TIMEOUT + time.Duration(rand.Intn(int(MAXELECTION_TIMEOUT-MINELECTION_TIMEOUT))))
		tmer := time.NewTimer(timeout)
		select {
		case <-tmer.C:
			//election timeout
			log.Println("Election timeout occurred for node:", r.me)
		}
	}
}
