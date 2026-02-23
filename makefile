USER         := tkt
PROJECT_DIR  := ~/proj/distsys/my_impl/raft
BINARY_NAME  := raft_server
CONFIG_FILE  := cluster.conf
LOG_DIR      := $(PROJECT_DIR)/logs

ALL_IDS := $(shell jq -r '.[].id' $(CONFIG_FILE))
TARGET_ID ?= all
ifeq ($(TARGET_ID),all)
    IDS := $(ALL_IDS)
else
    IDS := $(TARGET_ID)
endif

DEBUG ?= false
DEBUG_FLAG :=
ifeq ($(DEBUG),true)
    DEBUG_FLAG := --debug
endif

ASYNC_LOG ?= false
ASYNC_FLAG :=
ifeq ($(ASYNC_LOG),true)
    ASYNC_FLAG := --async-log
endif

ARGS ?=

# Client-side benchmark parameters
WORKERS     ?= 1 2 4 8 16 32
KEYS        ?= 6
TYPE        ?= ycsb-a

# Server-side benchmark parameters
READ_BATCH  ?= 1 2 4 8 16 32
WRITE_BATCH ?= 1 2 4 8 16 32

CLIENT_NODE := $(shell jq -r '.[0].id' $(CONFIG_FILE))
TIMESTAMP   := $(shell date +%Y%m%d_%H%M%S)

.PHONY: help deploy build send-bin start kill clean benchmark bench-tool-build-linux send-bench-tool bench-disk-remote bench-net-remote get-metrics

help:
	@echo "Usage: make [target] [options]"
	@echo ""
	@echo "Node targets:"
	@echo "  deploy         Distribute cluster.conf to all nodes"
	@echo "  send-bin       Cross-compile and send binary to all nodes"
	@echo "  start          Start Raft server nodes [TARGET_ID=id] [DEBUG=true] [ASYNC_LOG=true]"
	@echo "  kill           Kill Raft server processes [TARGET_ID=id]"
	@echo "  clean          Remove binaries and logs from nodes"
	@echo ""
	@echo "Benchmark:"
	@echo "  benchmark      Run full benchmark suite (server + client)"
	@echo "    TYPE         Workload type(s): ycsb-a ycsb-b ycsb-c  (default: ycsb-a)"
	@echo "    WORKERS      Client worker counts to sweep  (default: 1 2 4 8 16 32)"
	@echo "    KEYS         Number of keys  (default: 6)"
	@echo "    READ_BATCH   Server read batch sizes to sweep"
	@echo "    WRITE_BATCH  Server write batch sizes to sweep"
	@echo "    ASYNC_LOG    Enable async log writes (default: false)"
	@echo ""
	@echo "Infrastructure metrics:"
	@echo "  get-metrics"
	@echo "  bench-disk-remote   ID=<id>"
	@echo "  bench-net-remote    SERVER_ID=<id> CLIENT_ID=<id>"

deploy:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		echo "[$$ip] Distributing config..."; \
		scp $(CONFIG_FILE) $(USER)@$$ip:$(PROJECT_DIR)/$(notdir $(CONFIG_FILE)) & \
	done; wait

build:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Building $$bin..."; \
		ssh $(USER)@$$ip "cd $(PROJECT_DIR) && go build -o $$bin ./cmd" & \
	done; wait

send-bin:
	GOOS=linux GOARCH=amd64 go build -o /tmp/raft_tmp ./cmd
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Sending $$bin..."; \
		scp /tmp/raft_tmp $(USER)@$$ip:$(PROJECT_DIR)/$$bin && \
		ssh $(USER)@$$ip "chmod +x $(PROJECT_DIR)/$$bin" & \
	done; wait
	@rm /tmp/raft_tmp

start:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Starting $$bin (ID: $$id)..."; \
		ssh -n -f $(USER)@$$ip "mkdir -p $(LOG_DIR) && cd $(PROJECT_DIR) && \
		   (pkill -x $$bin || true) && \
		   sleep 0.5 && \
		   nohup ./$$bin start --id $$id --conf cluster.conf $(ARGS) $(DEBUG_FLAG) $(ASYNC_FLAG) > $(LOG_DIR)/node_$$id.ans 2>&1 < /dev/null &"; \
	done
	@echo "All start commands initiated."

kill:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Killing $$bin..."; \
		ssh $(USER)@$$ip "pkill -x $$bin || echo 'Not running.'" & \
	done; wait

clean:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Cleaning $$bin..."; \
		ssh $(USER)@$$ip "cd $(PROJECT_DIR) && rm -f $$bin logs/node_$$id.ans *.bin" results/* & \
	done; wait

# -----------------------------------------------------------------------
# benchmark: sweep server batch sizes × client workers, collect RESULT:
#
# Layout:
#   outer loops (type, rbatch, wbatch) → restart servers
#   inner loop  (workers)              → restart client only
# -----------------------------------------------------------------------
benchmark: send-bin
	@mkdir -p results
	@CLIENT_IP=$$(jq -r --arg i "$(CLIENT_NODE)" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
	CLIENT_BIN="$(BINARY_NAME)_$(CLIENT_NODE)"; \
	\
	for type in $(TYPE); do \
		BENCH_FILE="results/benchmark-$(TIMESTAMP)-$$type.csv"; \
		echo "Workload,ReadBatch,WriteBatch,Workers,Keys,Throughput(ops/sec),Latency(ms)" > "$$BENCH_FILE"; \
		echo "=== Workload: $$type ==="; \
		\
		for rbatch in $(READ_BATCH); do \
			for wbatch in $(WRITE_BATCH); do \
				echo "--- ReadBatch=$$rbatch WriteBatch=$$wbatch ---"; \
				\
				for id in $(ALL_IDS); do \
					ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
					ssh -n $(USER)@$$ip "rm -f $(LOG_DIR)/node_$$id.ans $(PROJECT_DIR)/raft_log_$$id.bin $(PROJECT_DIR)/raft_state_$$id.bin" & \
				done; wait; \
				\
				$(MAKE) kill; \
				sleep 1; \
				$(MAKE) start ARGS="--read-batch-size $$rbatch --write-batch-size $$wbatch $(ASYNC_FLAG)"; \
				sleep 8; \
				\
				for workers in $(WORKERS); do \
					echo "  Running client: workers=$$workers keys=$(KEYS)..."; \
					RES=$$(ssh $(USER)@$$CLIENT_IP \
						"cd $(PROJECT_DIR) && ./$$CLIENT_BIN client \
						--conf cluster.conf \
						--workers $$workers \
						--workload $$type \
						--keys $(KEYS)" \
					| grep 'RESULT:' | tail -n 1 | sed 's/.*RESULT://'); \
					\
					if [ -n "$$RES" ]; then \
						THROUGHPUT=$$(echo "$$RES" | cut -d, -f4); \
						LATENCY=$$(echo "$$RES"    | cut -d, -f5); \
						echo "$$type,$$rbatch,$$wbatch,$$workers,$(KEYS),$$THROUGHPUT,$$LATENCY" >> "$$BENCH_FILE"; \
						echo "  -> Throughput=$$THROUGHPUT ops/s  Latency=$$LATENCY ms"; \
					else \
						echo "  -> No result collected (client may have failed to reach leader)"; \
					fi; \
				done; \
				\
				$(MAKE) kill; \
				sleep 1; \
			done; \
		done; \
		echo "Results saved to $$BENCH_FILE"; \
	done
	@echo "All benchmarks finished."

# -----------------------------------------------------------------------
# Infrastructure metrics
# -----------------------------------------------------------------------
bench-tool-build-linux:
	cd disk_and_communication_latency_measure && GOOS=linux GOARCH=amd64 go build -o benchmark_linux main.go

send-bench-tool: bench-tool-build-linux
	@for id in $(ALL_IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		echo "[$$ip] Sending benchmark tool..."; \
		scp disk_and_communication_latency_measure/benchmark_linux $(USER)@$$ip:$(PROJECT_DIR)/benchmark_tool && \
		ssh $(USER)@$$ip "chmod +x $(PROJECT_DIR)/benchmark_tool" & \
	done; wait

bench-disk-remote:
	@if [ -z "$(ID)" ]; then echo "Usage: make bench-disk-remote ID=<id>"; exit 1; fi
	@IP=$$(jq -r --arg i "$(ID)" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
	echo "Running Disk Benchmark on Node $(ID) ($$IP)..."; \
	ssh $(USER)@$$IP "cd $(PROJECT_DIR) && ./benchmark_tool -mode disk"

bench-net-remote:
	@if [ -z "$(SERVER_ID)" ] || [ -z "$(CLIENT_ID)" ]; then echo "Usage: make bench-net-remote SERVER_ID=<id> CLIENT_ID=<id>"; exit 1; fi
	@echo "--- Setting up Network Benchmark between Node $(SERVER_ID) and Node $(CLIENT_ID) ---"
	@SERVER_IP=$$(jq -r --arg i "$(SERVER_ID)" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
	CLIENT_IP=$$(jq -r --arg i "$(CLIENT_ID)" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
	echo "Server: $$SERVER_IP | Client: $$CLIENT_IP"; \
	ssh -f $(USER)@$$SERVER_IP "cd $(PROJECT_DIR) && nohup ./benchmark_tool -mode server -id $(SERVER_ID) -config cluster.conf > /dev/null 2>&1 &"; \
	sleep 2; \
	ssh $(USER)@$$CLIENT_IP "cd $(PROJECT_DIR) && ./benchmark_tool -mode client -target $(SERVER_ID) -config cluster.conf"; \
	ssh $(USER)@$$SERVER_IP "pkill benchmark_tool || true && rm -f $(PROJECT_DIR)/benchmark_tool $(PROJECT_DIR)/../disk_and_communication_latency_measure/benchmark_linux"

get-metrics: send-bench-tool
	@NODE1=$$(jq -r '.[0].id' $(CONFIG_FILE)); \
	NODE2=$$(jq -r '.[1].id' $(CONFIG_FILE)); \
	echo "=== Disk Benchmark on Node $$NODE1 ==="; \
	$(MAKE) bench-disk-remote ID=$$NODE1; \
	if [ "$$NODE2" != "null" ]; then \
		echo ""; \
		echo "=== Network Benchmark (Server: Node $$NODE1, Client: Node $$NODE2) ==="; \
		$(MAKE) bench-net-remote SERVER_ID=$$NODE1 CLIENT_ID=$$NODE2; \
	fi
