package main

import (
	"github.com/pkg/errors"
	"net"
	"net/rpc"
)

func (r *Raft) initConns(peers []string) error {
	for _, peerID := range peers {
		if peerID != r.id && r.conns[peerID] == nil {
			client, err := rpc.Dial("tcp", peerID)
			if err != nil {
				return errors.WithStack(err)
			}
			r.conns[peerID] = client
		}
	}
	return nil
}

func (r *Raft) listenRPC() error {
	if err := rpc.Register(r); err != nil {
		return errors.WithStack(err)
	}
	l, err := net.Listen("tcp", r.id)
	if err != nil {
		return errors.WithStack(err)
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			return errors.WithStack(err)
		}
		go rpc.ServeConn(conn)
	}
}
