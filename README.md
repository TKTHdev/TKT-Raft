# TKT-Raft

[English](README.md) | [日本語](README.ja.md)

## Overview
- Simple implementation of Raft Consensus Algorithm
- Written in Go
- Usable as a Go library (`package raft`) with a pluggable `StateMachine` interface

## Features
- Leader election
- Log replication
- Safety (term, commit index, etc.)
- Pluggable state machine — bring your own `Apply`/`Query` implementation
- Built-in KV store (`KVStore`) for SET / GET / DELETE workloads
- Persistent storage (WAL for log, binary state file)
- Read-path optimization via quorum-read batching

---

### Directory Structure

```
raft/                  ← package raft  (library)
  raft.go              ← Config struct, New(), Raft struct
  consensus.go         ← Run(), election, replication loop
  rpc.go               ← RPC types and handlers
  handle_client.go     ← Request batching, Response type
  statemachine.go      ← StateMachine interface + KVStore
  storage.go           ← WAL / state persistence
  conns.go             ← TCP RPC listener & dialer
  config.go            ← cluster.conf parser (ParseConfig)
  logger.go            ← Debug logging
  cmd/                 ← package main  (binary)
    main.go            ← CLI entry point (urfave/cli)
    client.go          ← Benchmark client
```

---

### Key Modules

| File | Responsibility |
|---|---|
| `raft.go` | `Config`, `New()`, `Raft` struct |
| `consensus.go` | `Run()`, `doFollower`, `doLeader`, `startElection`, `processReadBatch` |
| `rpc.go` | `AppendEntries`, `RequestVote`, `Execute`, `Read` RPC handlers & senders |
| `handle_client.go` | `handleClientRequest` — batches writes to log, reads to quorum path |
| `statemachine.go` | `StateMachine` interface, `KVStore` implementation, `applyCommand` |
| `storage.go` | Binary WAL for log entries; binary state file for term/votedFor |
| `conns.go` | `listenRPC`, `dialRPCToPeer` |
| `config.go` | `ParseConfig` — reads `cluster.conf` JSON |

### StateMachine Interface

```go
type StateMachine interface {
    Apply(cmd []byte) []byte  // called after a log entry is committed
    Query(cmd []byte) []byte  // called after quorum confirmation (read path, no log)
}
```

Commands prefixed with `GET` are routed to the quorum-read path (`Query`); all others go through the Raft log (`Apply`).

---

## Using as a Library

```go
import "raft"

// Use the built-in KV store
node := raft.New(raft.Config{
    ID:       1,
    ConfPath: "cluster.conf",
}, raft.NewKVStore())
go node.Run()
```

Custom state machine example:

```go
type MembershipSM struct{ members map[int]string }

func (m *MembershipSM) Apply(cmd []byte) []byte {
    // handle ADD_MEMBER / REMOVE_MEMBER
    return nil
}
func (m *MembershipSM) Query(cmd []byte) []byte {
    // return current member list
    return nil
}

node := raft.New(raft.Config{
    ID:       myID,
    ConfPath: "raft.conf",
}, &MembershipSM{members: make(map[int]string)})
go node.Run()
```

---

## Building & Running

### Build the binary

```bash
go build -o raft_server ./cmd
```

### Run a single node

```bash
./raft_server start --id 1 --conf cluster.conf
```

### Available flags

| Flag | Default | Description |
|---|---|---|
| `--id` | (required) | Node ID |
| `--conf` | `cluster.conf` | Path to config file |
| `--write-batch-size` | `128` | Max log entries batched per fsync |
| `--read-batch-size` | `128` | Max reads batched per quorum round |
| `--debug` | `false` | Enable coloured debug logging |
| `--async-log` | `false` | Skip fsync on each write (faster, less durable) |

---

### Cluster Management via Makefile

The `makefile` automates deployment over SSH.

**Prerequisites:**
1. Password-less SSH access to all IPs in `cluster.conf`.
2. Update `USER` and `PROJECT_DIR` at the top of `makefile`.

**Core Commands:**

| Command | Description |
|---|---|
| `make deploy` | Distribute `cluster.conf` to all nodes |
| `make send-bin` | Cross-compile (Linux/AMD64) and push binary to all nodes |
| `make build` | Build on remote nodes (requires Go installed there) |
| `make start` | Start Raft server on all nodes (logs → `logs/node_<ID>.ans`) |
| `make kill` | Stop Raft server processes on all nodes |
| `make clean` | Remove binaries and logs from nodes |
| `make benchmark` | Sweep workload × batch sizes × worker counts, output CSV |
| `make get-metrics` | Measure disk and network latency of cluster nodes |

**Example workflow:**

```bash
make deploy       # push cluster.conf
make send-bin     # push binary
make start        # start all nodes
make benchmark TYPE=ycsb-a WORKERS="1 4 16" READ_BATCH="1 32" WRITE_BATCH="1 32"
make kill
```

**Manual start (debugging):**

```bash
./raft_server start --id 1 --conf cluster.conf  # terminal 1
./raft_server start --id 2 --conf cluster.conf  # terminal 2
./raft_server start --id 3 --conf cluster.conf  # terminal 3
```

---

## Limitations

1. **Static membership** — cluster size is fixed at startup via `cluster.conf`.
2. **No log compaction** — the log grows indefinitely; no snapshotting.
3. **KV read routing is prefix-based** — commands starting with `GET` go through the quorum-read path; custom state machines that need `Query` must follow the same convention.
4. **No external API** — interaction is via Go RPC (`net/rpc`) only.
