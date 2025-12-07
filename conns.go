package main

import (
	"net"
	"net/rpc"

	"github.com/pkg/errors"
)

func (r *Raft) initConns(peerIPPort map[int]string) error {
	for {
		for idx, peerID := range peerIPPort {
			if idx != r.me && r.rpcConns[idx] == nil {
				client, err := rpc.Dial("tcp", peerID)
				if err != nil {
					continue
				}
				r.rpcConns[idx] = client
			}
		}
	}
	return nil
}

func (r *Raft) listenRPC(peerIPPort map[int]string) error {
	if err := rpc.Register(r); err != nil {
		return errors.WithStack(err)
	}
	l, err := net.Listen("tcp", peerIPPort[r.me])
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
