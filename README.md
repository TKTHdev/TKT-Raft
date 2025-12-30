# TKT-Raft

## Overview
- Simple implementation of Raft Consensus Algorithm
- Written in Go

## Features
- What are implemented
    - Leader election
    - Log replication
    - Safety (term, commit index, etc.)
- What are not implemented
    - Linearizable Read
    - Persistent State
        - Term
        - Log
        - VotedFor


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

### Run a Local Cluster (3 Nodes)
You can run a 3-node cluster on your local machine by spawning three separate processes. Alternatively, you can use the `controller/makefile` for easier management.

**Using the `controller/makefile`:**

1.  **Ensure `cluster.conf` is correct:**
    ```json
    [
      { "id": 1, "ip": "localhost", "port": 5000},
      { "id": 2, "ip": "localhost", "port": 5001},
      { "id": 3, "ip": "localhost", "port": 5002}
    ]
    ```

2.  **Build and Start the nodes:**
    ```bash
    cd controller
    make build    # Builds raft_server_1, raft_server_2, raft_server_3
    make start    # Starts all nodes in the background
    ```

3.  **To stop the cluster:**
    ```bash
    cd controller
    make kill
    ```

**Manual Start (for individual control or debugging):**

1.  **Ensure `cluster.conf` is correct:**
    ```json
    [
      { "id": 1, "ip": "localhost", "port": 5000},
      { "id": 2, "ip": "localhost", "port": 5001},
      { "id": 3, "ip": "localhost", "port": 5002}
    ]
    ```

2.  **Start the nodes:**
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
*   **Scenarios:** The implementation implicitly tests **Leader Election** (via timeouts in `consensus.go`) and **Replication** (via the internal client pushing logs). The `controller/makefile` suggests a workflow for deploying to multiple hosts to test real network distribution.

## Limitations

1.  **No Persistence:** If a node crashes and restarts, it loses all log entries and state. `currentTerm`, `votedFor`, and the `log` are not saved to disk.
2.  **Fixed Configuration:** Cluster membership is static, defined in `cluster.conf` at startup. Dynamic membership changes are not supported.
3.  **No Snapshotting:** The log will grow indefinitely. There is no mechanism to compact the log or create snapshots.
4.  **Internal-Only Client:** External applications cannot interact with the cluster. The `client` logic is hardcoded within the server binary.
5.  **Basic Error Handling:** Network errors are logged, but complex recovery scenarios or backoff strategies might be minimal.
6.  **TODOs (Inferred):**
    *   Implement persistent storage (WAL).
    *   Add an external API (HTTP/gRPC) for client interaction.
    *   Implement log compaction/snapshots.
    *   Add formal unit tests.
