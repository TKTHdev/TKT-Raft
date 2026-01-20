USER         := tkt
PROJECT_DIR  := ~/proj/raft
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

WORKERS ?= 1 2 4 8 16 32
READ_BATCH ?= 1 2 4 8 16 32
WRITE_BATCH ?= 1 2 4 8 16 32
TYPE    ?= ycsb-a
TIMESTAMP := $(shell date +%Y%m%d_%H%M%S)

.PHONY: help deploy build send-bin start kill clean benchmark bench-tool-build-linux send-bench-tool bench-disk-remote bench-net-remote get-metrics

help:
	@echo "Usage: make [target] [TARGET_ID=id] [DEBUG=true] [ASYNC_LOG=true]"
	@echo "Targets: deploy, build, send-bin, start, kill, clean, benchmark"
	@echo "Benchmark Tools:"
	@echo "  make send-bench-tool"
	@echo "  make bench-disk-remote ID=<id>"
	@echo "  make bench-net-remote SERVER_ID=<id> CLIENT_ID=<id>"
	@echo "  make get-metrics"

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
		ssh $(USER)@$$ip "cd $(PROJECT_DIR) && go build -o $$bin" & \
	done; wait

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
	echo "Starting Benchmark Server on Node $(SERVER_ID)..."; \
	ssh -f $(USER)@$$SERVER_IP "cd $(PROJECT_DIR) && nohup ./benchmark_tool -mode server -id $(SERVER_ID) -config cluster.conf > /dev/null 2>&1 &"; \
	sleep 2; \
	echo "Running Benchmark Client on Node $(CLIENT_ID)..."; \
	ssh $(USER)@$$CLIENT_IP "cd $(PROJECT_DIR) && ./benchmark_tool -mode client -target $(SERVER_ID) -config cluster.conf"; \
	echo "Stopping Benchmark Server..."; \
	ssh $(USER)@$$SERVER_IP "pkill benchmark_tool || true && cd $(PROJECT_DIR) && rm benchmark_tool && rm disk_and_communication_latency_measure/benchmark_linux"

get-metrics: send-bench-tool
	@echo "Selecting nodes for metrics..."
	@NODE1=$$(jq -r '.[0].id' $(CONFIG_FILE)); \
	NODE2=$$(jq -r '.[1].id' $(CONFIG_FILE)); \
	echo "=== Running Disk Benchmark on Node $$NODE1 ==="; \
	$(MAKE) bench-disk-remote ID=$$NODE1; \
	if [ "$$NODE2" != "null" ]; then \
		echo ""; \
		echo "=== Running Network Benchmark (Server: Node $$NODE1, Client: Node $$NODE2) ==="; \
		$(MAKE) bench-net-remote SERVER_ID=$$NODE1 CLIENT_ID=$$NODE2; \
	else \
		echo "Not enough nodes for network benchmark."; \
	fi



send-bin:
	GOOS=linux GOARCH=amd64 go build -o /tmp/raft_tmp .
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

benchmark:
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
						ssh -n $(USER)@$$ip "rm -f $(LOG_DIR)/node_$$id.ans"; \
						if [ "$$type" != "ycsb-c" ]; then \
							ssh -n $(USER)@$$ip "cd $(PROJECT_DIR) && rm -f raft_log_$$id.bin raft_state_$$id.bin"; \
						fi; \
					done; \
					\
					$(MAKE) kill; \
					sleep 2; \
					$(MAKE) start ARGS="--read-batch-size $$rbatch --write-batch-size $$wbatch --workers $$workers --workload $$type $(ASYNC_FLAG)"; \
					sleep 20; \
					\
					echo "--- Collecting results for Type=$$type, Workers=$$workers ---"; \
					\
					for id in $(IDS); do \
						ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
						\
						RES=$$(ssh -n $(USER)@$$ip "grep 'RESULT:' $(LOG_DIR)/node_$$id.ans | tail -n 1" | sed 's/.*RESULT://' | awk -F, '{print $$(NF-1) "," $$NF}' | tr -d ' \r\n'); \
						\
						if [ -n "$$RES" ]; then \
							echo "$$type,$$rbatch,$$wbatch,$$workers,$$RES" >> "$$BENCH_FILE"; \
						fi; \
					done; \
				done; \
			done; \
		done; \
		echo "Finished workload: $$type. Results saved to $$BENCH_FILE"; \
	done
	@$(MAKE) kill
	@echo "All benchmarks finished."
