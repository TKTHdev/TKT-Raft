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

ARGS ?=

.PHONY: help deploy build send-bin start kill benchmark

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
	GOOS=linux GOARCH=amd64 go build -o /tmp/raft_tmp ..
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
	      nohup ./$$bin start --id $$id --conf cluster.conf $(ARGS) > $(LOG_DIR)/node_$$id.ans 2>&1 < /dev/null &"; \
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
		ssh $(USER)@$$ip "cd $(PROJECT_DIR) && rm $$bin || echo 'Binary not existing' && rm logs/node_$$id.ans || echo 'Log not existing' && rm *.bin || echo 'Raft log not existing'" & \
	done; wait

benchmark:
	@mkdir -p results
	@echo "Starting benchmark..." > results/benchmark.log
	@for batch in 32 64; do \
		for workers in 256; do \
			echo "Running benchmark: Batch=$$batch, Workers=$$workers"; \
			$(MAKE) kill; \
			sleep 2; \
			$(MAKE) start ARGS="--batch-size $$batch --workers $$workers"; \
			sleep 20; \
			echo "--- Results for Batch=$$batch, Workers=$$workers ---" >> results/benchmark.log; \
			grep "ConcClient throughput" $(LOG_DIR)/*.ans >> results/benchmark.log || echo "No throughput data found" >> results/benchmark.log; \
		done; \
	done
	@$(MAKE) kill
	@echo "Benchmark finished. Results saved to results/benchmark.log"