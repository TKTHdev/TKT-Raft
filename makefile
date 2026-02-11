# ==============================================================================
# Variables
# ==============================================================================

# Application info
BINARY_NAME := raft_bin

# Configuration
CONFIG_FILE := cluster.conf
USER        := tkt
REMOTE_DIR  := ~/proj/distsys/my_impl/raft
LOG_DIR     := logs

# Benchmark / Runtime args
DEBUG       ?= false
ASYNC_LOG   ?= false
TARGET_ID   ?= all

# Helper variables for flags
DEBUG_FLAG :=
ifeq ($(DEBUG),true)
    DEBUG_FLAG := --debug
endif

ASYNC_FLAG :=
ifeq ($(ASYNC_LOG),true)
    ASYNC_FLAG := --async-log
endif

# Benchmark Params
WORKERS     ?= 1 2 4 8 16 32
READ_BATCH  ?= 1 2 4 8 16 32
WRITE_BATCH ?= 1 2 4 8 16 32
TYPE        ?= ycsb-a
TIMESTAMP   := $(shell date +%Y%m%d_%H%M%S)

# JQ extraction (requires jq)
ALL_IDS := $(shell jq -r '.[].id' $(CONFIG_FILE) 2>/dev/null || echo "")
ifeq ($(TARGET_ID),all)
    IDS := $(ALL_IDS)
else
    IDS := $(TARGET_ID)
endif

# ==============================================================================
# Standard Go Targets (Local)
# ==============================================================================

.PHONY: all build test clean fmt vet tidy help

all: build

# Build the binary locally
build:
	go build -o $(BINARY_NAME) .

# Run tests
test:
	go test -v ./...

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...

# Tidy modules
tidy:
	go mod tidy

# Clean local artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -f raft_log_*.bin raft_state_*.bin
	rm -f *.log *.ans
	rm -rf $(LOG_DIR)

help:
	@echo "Usage: make [target] [TARGET_ID=id] [DEBUG=true] [ASYNC_LOG=true]"
	@echo ""
	@echo "Local Development:"
	@echo "  make build          Build binary locally"
	@echo "  make test           Run tests"
	@echo "  make clean          Clean local files"
	@echo "  make local-start    Start 3-node cluster locally"
	@echo "  make local-client   Run YCSB benchmark locally"
	@echo "  make local-stop     Stop local cluster"
	@echo ""
	@echo "Remote Orchestration:"
	@echo "  make deploy         Send config to remote nodes"
	@echo "  make send-bin       Build Linux binary and send to remote"
	@echo "  make remote-start   Start remote cluster"
	@echo "  make remote-kill    Stop remote cluster"
	@echo "  make benchmark      Run full benchmark suite"

# ==============================================================================
# Local Cluster Development
# ==============================================================================

.PHONY: local-start local-stop local-client local-test

local-start: build clean
	@echo "Starting 3-node Raft cluster (localhost)..."
	@mkdir -p $(LOG_DIR)
	@./$(BINARY_NAME) start --id 1 --conf $(CONFIG_FILE) --async-log $(DEBUG_FLAG) > $(LOG_DIR)/raft1.log 2>&1 &
	@./$(BINARY_NAME) start --id 2 --conf $(CONFIG_FILE) --async-log $(DEBUG_FLAG) > $(LOG_DIR)/raft2.log 2>&1 &
	@./$(BINARY_NAME) start --id 3 --conf $(CONFIG_FILE) --async-log $(DEBUG_FLAG) > $(LOG_DIR)/raft3.log 2>&1 &
	@echo "Waiting 5s for leader election..."
	@sleep 5
	@echo "Cluster ready. Logs in $(LOG_DIR)/"

local-stop:
	@pkill -f $(BINARY_NAME) 2>/dev/null || true
	@echo "Cluster stopped."

LOCAL_WORKERS  ?= 256
LOCAL_WORKLOAD ?= ycsb-a

local-client: build
	@echo "Running YCSB (workers=$(LOCAL_WORKERS), workload=$(LOCAL_WORKLOAD)) against all ports..."
	@./$(BINARY_NAME) client --write-addr localhost:7000 --workers $(LOCAL_WORKERS) --workload $(LOCAL_WORKLOAD) > $(LOG_DIR)/client_c0.log 2>&1 &
	 ./$(BINARY_NAME) client --write-addr localhost:7001 --workers $(LOCAL_WORKERS) --workload $(LOCAL_WORKLOAD) > $(LOG_DIR)/client_c1.log 2>&1 &
	 ./$(BINARY_NAME) client --write-addr localhost:7002 --workers $(LOCAL_WORKERS) --workload $(LOCAL_WORKLOAD) > $(LOG_DIR)/client_c2.log 2>&1 &
	 wait
	@grep "^RESULT:" $(LOG_DIR)/client_*.log 2>/dev/null | grep -v ",0.00,0.00$$" || echo "(no leader found)"

local-test: local-stop local-start
	@sleep 2
	$(MAKE) local-client
	$(MAKE) local-stop

# ==============================================================================
# Remote Deployment & Orchestration
# ==============================================================================

.PHONY: deploy remote-build send-bin remote-start remote-kill remote-clean benchmark

# Deploy config to remote nodes
deploy:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		echo "[$$ip] Distributing config..."; \
		scp $(CONFIG_FILE) $(USER)@$$ip:$(REMOTE_DIR)/$(notdir $(CONFIG_FILE)) & \
	done; wait

# Build directly on remote nodes (if they have Go installed)
remote-build:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Building $$bin..."; \
		ssh $(USER)@$$ip "cd $(REMOTE_DIR) && go build -o $$bin" & \
	done; wait

# Cross-compile locally and send to remote
send-bin:
	GOOS=linux GOARCH=amd64 go build -o /tmp/raft_tmp .
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Sending $$bin..."; \
		scp /tmp/raft_tmp $(USER)@$$ip:$(REMOTE_DIR)/$$bin && \
		ssh $(USER)@$$ip "chmod +x $(REMOTE_DIR)/$$bin" & \
	done; wait
	@rm /tmp/raft_tmp

remote-start:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Starting $$bin (ID: $$id)..."; \
		ssh -n -f $(USER)@$$ip "cd $(REMOTE_DIR) && mkdir -p $(LOG_DIR) && \
		   (pkill -x $(BINARY_NAME)_$$id || true) && \
		   sleep 0.5 && \
		   nohup ./$(BINARY_NAME)_$$id start --id $$id --conf cluster.conf $(ARGS) $(DEBUG_FLAG) $(ASYNC_FLAG) > $(REMOTE_DIR)/$(LOG_DIR)/node_$$id.ans 2>&1 < /dev/null &"; \
	done
	@echo "All start commands initiated."

remote-kill:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Killing $$bin..."; \
		ssh $(USER)@$$ip "pkill -x $(BINARY_NAME)_$$id || echo 'Not running.'" & \
	done; wait

remote-clean:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Cleaning $$bin..."; \
		ssh $(USER)@$$ip "cd $(REMOTE_DIR) && rm -f $$bin logs/node_$$id.ans *.bin" results/* & \
	done; wait

# ==============================================================================
# Benchmarking Tools
# ==============================================================================

.PHONY: benchmark

# Remote benchmark: starts servers, then runs internal client (based on tsujido) from one node
benchmark:
	$(MAKE) send-bin
	@mkdir -p results
	@echo "Starting benchmark..."
	@for type in $(TYPE); do \
		BENCH_FILE="results/benchmark-$(TIMESTAMP)-$$type.csv"; \
		echo "Initializing $$BENCH_FILE ..."; \
		echo "Workload,ReadBatch,WriteBatch,Workers,Throughput(ops/sec),Latency(ms)" > "$$BENCH_FILE"; \
		\
		for rbatch in $(READ_BATCH); do \
			for wbatch in $(WRITE_BATCH); do \
				for workers in $(WORKERS); do \
					echo "Running benchmark: Type=$$type, ReadBatch=$$rbatch, WriteBatch=$$wbatch, Workers=$$workers"; \
					\
					for id in $(IDS); do \
						ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
						ssh -n $(USER)@$$ip "rm -f $(REMOTE_DIR)/$(LOG_DIR)/node_$$id.ans"; \
						if [ "$$type" != "ycsb-c" ]; then \
							ssh -n $(USER)@$$ip "cd $(REMOTE_DIR) && rm -f raft_log_$$id.bin raft_state_$$id.bin"; \
						fi; \
						done; \
					\
					$(MAKE) remote-kill; \
					sleep 2; \
					$(MAKE) remote-start ARGS="--read-batch-size $$rbatch --write-batch-size $$wbatch $(ASYNC_FLAG)"; \
					sleep 20; \
					\
					echo "--- Running client for Type=$$type, Workers=$$workers ---"; \
					\
					for id in $(IDS); do \
						ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
						client_port=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .client_port' $(CONFIG_FILE)); \
						ssh -n $(USER)@$$ip "cd $(REMOTE_DIR) && mkdir -p $(LOG_DIR) && \
						   nohup ./$(BINARY_NAME)_$$id client --write-addr $$ip:$$client_port --workers $$workers --workload $$type \
						   > $(REMOTE_DIR)/$(LOG_DIR)/client_$$id.ans 2>&1 < /dev/null" & \
					done; wait; \
					\
					echo "--- Collecting results ---"; \
					\
					for id in $(IDS); do \
						ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
						\
						RES=$$(ssh -n $(USER)@$$ip "grep 'RESULT:' $(REMOTE_DIR)/$(LOG_DIR)/client_$$id.ans | tail -n 1" | sed 's/.*RESULT://' | awk -F, '{print $$(NF-1) "," $$NF}' | tr -d ' \r\n'); \
						\
						if [ -n "$$RES" ] && ! echo "$$RES" | grep -q "0.00,0.00$$"; then \
							echo "$$type,$$rbatch,$$wbatch,$$workers,$$RES" >> "$$BENCH_FILE"; \
						fi; \
					done; \
				done; \
			done; \
		done; \
		echo "Finished workload: $$type. Results saved to $$BENCH_FILE"; \
	done
	@$(MAKE) remote-kill
	@echo "All benchmarks finished."