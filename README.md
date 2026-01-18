# TKT-Raft

[English](README.md) | [日本語](README.ja.md)

## Overview
- Simple implementation of Raft Consensus Algorithm
- Written in Go

## Features
- What are implemented
    - Leader election
    - Log replication
    - Safety (term, commit index, etc.)


### Raft Node Overview
The core of the system is the `Raft` struct defined in `raft.go`. Each node operates as a standalone server that communicates with peers via RPC.
- **State Management:** The node maintains standard Raft state: `currentTerm`, `votedFor`, `log`, `commitIndex`, and `lastApplied`.
- **Roles:** The node transitions between `FOLLOWER`, `CANDIDATE`, and `LEADER` states, managed by the main event loop in `consensus.go`.

### Key Modules & Relationships
*   **`raft.go`**: Defines the `Raft` struct and initializes the node (`NewRaft`). It holds the central state, including the log and RPC connections.
*   **`consensus.go`**: Contains the main `Run()` loop. It handles the core lifecycle:
    *   **Follower:** Waits for heartbeats or election timeout (`doFollower`).
    *   **Leader:** Sends periodic heartbeats (`doLeader`) and updates the `commitIndex`.
    *   **Candidate:** Manages the election process (`startElection`).
*   **`rpc.go`**: Implements the Raft RPC handlers (`RequestVote`, `AppendEntries`) and the client-side logic to send these RPCs to peers.
*   **`replication.go`**: Handles the ingestion of new commands from the internal client (`handleClientRequest`) and appends them to the local log.
*   **`statemachine.go`**: Implements the application state logic (`applyCommand`). It parses commands (`SET`, `GET`, `DELETE`) and updates an in-memory `map[string]string`.
*   **`conns.go`**: Manages network connectivity. It handles TCP listeners (`listenRPC`) and establishes outgoing RPC connections (`dialRPCToPeer`).
*   **`config.go`**: Parses the `cluster.conf` file to map Node IDs to IP:Port addresses.
*   **`client.go`**: An **internal load generator** that simulates client traffic by randomly creating commands and sending them to the leader's processing channel.

### Storage & Transport Abstractions
*   **Storage:** **In-Memory Only.** The Raft log (`[]LogEntry`) and the State Machine (`map[string]string`) are stored entirely in memory within the `Raft` struct. There is no persistence to disk implemented in the provided code (no WAL or snapshotting).
*   **Transport:** **Go `net/rpc`.** The implementation uses Go's standard `net/rpc` package over TCP. Nodes dial each other based on the addresses defined in `cluster.conf`.

## How to build and run

### Build
You can build the project using the standard Go toolchain.
```bash
go build -o raft_server .
```

### Run a Single Node
To run a single node locally, you need a valid `cluster.conf` (one is provided in the root) and pass the node ID.

```bash
# Assuming cluster.conf contains: [{"id": 1, "ip": "localhost", "port": 5000}]
./raft_server start --id 1 --conf cluster.conf
```

### Cluster Management via Makefile
The included `makefile` automates the deployment, building, and lifecycle management of the cluster using `ssh` and `scp`. It is designed to work with the nodes defined in `cluster.conf`.

**Prerequisites:**
1.  **SSH Access:** You must have password-less SSH access to all IPs listed in `cluster.conf` (including `localhost`). It is recommended to use `ssh-agent` and `ssh-add` so that the Makefile can execute commands on remote nodes without prompting for passwords.
2.  **Configuration:**
    *   **`cluster.conf`**: Define your nodes (ID, IP, Port).
    *   **`makefile`**: Open the file and update `USER` (default: `tkt`) and `PROJECT_DIR` (default: `~/proj/raft`) to match your environment.

**Core Commands:**

*   **`make deploy`**
    Distributes the `cluster.conf` file to all nodes listed in the config.

*   **`make send-bin`**
    Cross-compiles the binary locally (Linux/AMD64) and transfers it to all remote nodes. This is the recommended way to update code.

*   **`make build`**
    Triggers a `go build` command *on* the remote nodes. Use this if the remote nodes have Go installed and you prefer remote compilation.

*   **`make start`**
    Starts the Raft server on all nodes in the background. Logs are redirected to `logs/node_<ID>.ans`.

*   **`make kill`**
    Stops the Raft server processes on all nodes.

*   **`make clean`**
    Removes binaries and log files from the nodes.

**Benchmarking & Metrics:**

*   **`make benchmark`**
    Runs a comprehensive benchmark suite measuring throughput and latency across different workloads and batch sizes.

*   **`make get-metrics`**
    Runs `bench-disk-remote` and `bench-net-remote` to measure the underlying disk and network performance of your cluster environment.

**Example Workflow:**

1.  **Configure:** Edit `cluster.conf` and `makefile` variables.
2.  **Deploy Config:** `make deploy`
3.  **Update Code:** `make send-bin`
4.  **Start Cluster:** `make start`
5.  **Monitor:** Check logs on nodes (e.g., `tail -f logs/node_1.ans`).
6.  **Stop:** `make kill`

**Manual Start (Debugging):**
If you prefer not to use the makefile or SSH, you can run nodes manually in separate terminals:

```bash
# Terminal 1
./raft_server start --id 1 --conf cluster.conf

# Terminal 2
./raft_server start --id 2 --conf cluster.conf

# Terminal 3
./raft_server start --id 3 --conf cluster.conf
```

### Minimal Sample Code
The project is designed as a standalone binary rather than a library, but here is how the `main` function essentially bootstraps a node (based on `init.go`):

```go
package main

func main() {
    // 1. Define configuration path and Node ID
    nodeID := 1
    confPath := "cluster.conf"

    // 2. Initialize the Raft instance
    raftNode := NewRaft(nodeID, confPath)

    // 3. Start the main event loop (blocks forever)
    raftNode.Run()
}
```

## API / Usage

### Entry Point
The entry point is the CLI command defined in `init.go`, utilizing `urfave/cli`.
*   Command: `start`
*   Flags: `--id <int>`, `--conf <string>`

### Application Interface
*   **No external API:** There is no HTTP or gRPC interface for external clients to submit commands.
*   **Internal Client:** The "usage" is currently simulated by `client.go`, which runs a goroutine inside the `Raft` process. It generates random `SET`, `GET`, and `DELETE` commands and sends them to the `ClientCh`.
*   **State Machine Hook:** If you were to modify this for a real application, you would edit `statemachine.go`:
    ```go
    // Inside statemachine.go
    func (r *Raft) applyCommand(command []byte) {
        // Parse your command here
        // Update your application state
    }
    ```

## Testing

### Strategy
*   **Simulation/Load Testing:** The project relies on the internal `client.go` to generate random traffic (`randomOperation`, `createRandomCommand`). This acts as a continuous integration test when the cluster is running.
*   **Unit Tests:** There are no standard Go unit tests (`_test.go` files) visible in the top-level directory.
*   **Scenarios:** The implementation implicitly tests **Leader Election** (via timeouts in `consensus.go`) and **Replication** (via the internal client pushing logs). The `makefile` suggests a workflow for deploying to multiple hosts to test real network distribution.

## Limitations


1.  **Fixed Configuration:** Cluster membership is static, defined in `cluster.conf` at startup. Dynamic membership changes are not supported.
2.  **No Snapshotting:** The log will grow indefinitely. There is no mechanism to compact the log or create snapshots.
3.  **Internal-Only Client:** External applications cannot interact with the cluster. The `client` logic is hardcoded within the server binary.
4.  **Basic Error Handling:** Network errors are logged, but complex recovery scenarios or backoff strategies might be minimal.
5.  **TODOs (Inferred):**
    *   Add an external API (HTTP/gRPC) for client interaction.
    *   Implement log compaction/snapshots.
    *   Add formal unit tests.
