package main

import (
	"fmt"
	"net"
	"net/rpc"

	"github.com/pkg/errors"
)

func (r *Raft) dialRPCToPeer(peerID int) error {
	if peerID == r.me {
		return nil
	}
	client, err := rpc.Dial("tcp", r.peerIPPort[peerID])
	if err != nil {
		logMsg := fmt.Sprintf("Failed to connect to peer %d at %s: %v", peerID, r.peerIPPort[peerID], err)
		r.logPut(logMsg, PURPLE)
		return errors.WithStack(err)
	}
	r.rpcConns[peerID] = client
	msg := fmt.Sprintf("Connected to peer %d at %s", peerID, r.peerIPPort[peerID])
	r.logPut(msg, GREEN)
	return nil
}

func (r *Raft) dialRPCToAllPeers() error {
	for peerID := range r.peerIPPort {
		if peerID != r.me {
			logMsg := fmt.Sprintf("Dialing RPC to peer %d at %s", peerID, r.peerIPPort[peerID])
			r.logPut(logMsg, CYAN)
			go r.dialRPCToPeer(peerID)
		}
	}
	return nil
}

func (r *Raft) listenRPC(peerIPPort map[int]string) error {
	_ = rpc.Register(r)
	l, err := net.Listen("tcp", peerIPPort[r.me])
	if err != nil {
		return errors.WithStack(err)
	}
	msg := fmt.Sprintf("Listening for RPC connections on %s", peerIPPort[r.me])
	r.logPut(msg, PURPLE)
	for {
		logMsg := fmt.Sprintf("Waiting to accept RPC connection on %s", peerIPPort[r.me])
		r.logPut(logMsg, CYAN)
		conn, err := l.Accept()
		if err != nil {
			logMsg := fmt.Sprintf("Failed to accept RPC connection: %v", err)
			r.logPut(logMsg, PURPLE)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
