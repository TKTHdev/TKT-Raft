package main

import (
	"log"
	"net"
	"net/rpc"

	"github.com/pkg/errors"
)

func (r *Raft) initConns() error {
	for {
		for idx, peerID := range r.peerIPPort {
			if idx != r.me && r.rpcConns[idx] == nil {
				log.Println("Connecting to peer:", peerID)
				client, err := rpc.Dial("tcp", peerID)
				if err != nil {
					log.Println("Failed to connect to peer:", peerID, "Error:", err)
					continue
				}
				r.rpcConns[idx] = client
				log.Println("Connected to peer:", peerID)
			}
		}
	}
	return nil
}

func (r *Raft) listenRPC() error {
	if err := rpc.Register(r); err != nil {
		return errors.WithStack(err)
	}
	l, err := net.Listen("tcp", r.peerIPPort[r.me])
	if err != nil {
		return errors.WithStack(err)
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			return errors.WithStack(err)
		}
		go rpc.ServeConn(conn)
		log.Println("Accepted connection from:", conn.RemoteAddr())
	}
}
