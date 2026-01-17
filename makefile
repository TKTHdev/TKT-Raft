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

ARGS ?=

WORKERS ?= 1 2 4 8 16 32
BATCH   ?= 1 2 4 8 16 32
TYPE    ?= ycsb-a
TIMESTAMP := $(shell date +%Y%m%d_%H%M%S)

.PHONY: help deploy build send-bin start kill clean benchmark

help:
	@echo "Usage: make [target] [TARGET_ID=id] [DEBUG=true]"
	@echo "Targets: deploy, build, send-bin, start, kill, clean, benchmark"

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
		   nohup ./$$bin start --id $$id --conf cluster.conf $(ARGS) $(DEBUG_FLAG) > $(LOG_DIR)/node_$$id.ans 2>&1 < /dev/null &"; \
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
		ssh $(USER)@$$ip "cd $(PROJECT_DIR) && rm -f $$bin logs/node_$$id.ans *.bin" & \
	done; wait

benchmark:
	@mkdir -p results
	@echo "Workload,Batch,Workers,Throughput(ops/sec),Latency(ms)" > results/benchmark-$(TIMESTAMP).csv
	@echo "Starting benchmark..."
	@for type in $(TYPE); do \
		for batch in $(BATCH); do \
			for workers in $(WORKERS); do \
				echo "Running benchmark: Type=$$type, Batch=$$batch, Workers=$$workers"; \
				for id in $(IDS); do \
					ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
					ssh -n $(USER)@$$ip "rm -f $(LOG_DIR)/node_$$id.ans"; \
					if [ "$$type" != "ycsb-c" ]; then \
						ssh -n $(USER)@$$ip "cd $(PROJECT_DIR) && rm -f raft_log_$$id.bin"; \
						ssh -n $(USER)@$$ip "cd $(PROJECT_DIR) && rm -f raft_state_$$id.bin"; \
					fi; \
				done; \
				$(MAKE) kill; \
				sleep 2; \
				$(MAKE) start ARGS="--batch-size $$batch --workers $$workers --workload $$type"; \
				sleep 20; \
				echo "--- Results for Type=$$type, Batch=$$batch, Workers=$$workers ---"; \
				for id in $(IDS); do \
					ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
					ssh -n $(USER)@$$ip "grep 'RESULT:' $(LOG_DIR)/node_$$id.ans | cut -d':' -f2" >> results/benchmark-$(TIMESTAMP)-$$type.csv; \
				done; \
			done; \
		done; \
	done
	@$(MAKE) kill
	@echo "Benchmark finished. Results saved to results/benchmark-$(TIMESTAMP).csv"
