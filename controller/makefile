# --- 設定項目 ---
USER         := tkt
PROJECT_DIR  := ~/study/raft
BINARY_NAME  := raft_server
CONFIG_FILE  := ../cluster.conf
LOG_DIR      := $(PROJECT_DIR)/logs

# --- 内部変数 ---
ALL_IDS := $(shell jq -r '.[].id' $(CONFIG_FILE))
TARGET_ID ?= all
ifeq ($(TARGET_ID),all)
    IDS := $(ALL_IDS)
else
    IDS := $(TARGET_ID)
endif

.PHONY: help deploy build send-bin start kill

# 1. 設定ファイルの配布
deploy:
	@for id in $(IDS); do \
	   ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
	   echo "[$$ip] Distributing config..."; \
	   scp $(CONFIG_FILE) $(USER)@$$ip:$(PROJECT_DIR)/$(notdir $(CONFIG_FILE)) & \
	done; wait

# 2. リモートビルド (raft_server_$id を作成)
build:
	@for id in $(IDS); do \
	   ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
	   bin="$(BINARY_NAME)_$$id"; \
	   echo "[$$ip] Building $$bin..."; \
	   ssh $(USER)@$$ip "cd $(PROJECT_DIR) && go build -o $$bin" & \
	done; wait

# 3. ローカルビルド & 配布
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

# 4. Raftの起動
# [Suy luận] pkill -x を使うことで、バイナリ名に完全一致するプロセスのみを対象にします。
start:
	@for id in $(IDS); do \
	   ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
	   bin="$(BINARY_NAME)_$$id"; \
	   echo "[$$ip] Starting $$bin (ID: $$id)..."; \
	   ssh -n -f $(USER)@$$ip "mkdir -p $(LOG_DIR) && cd $(PROJECT_DIR) && \
	      (pkill -x $$bin || true) && \
	      sleep 0.5 && \
	      nohup ./$$bin start --id $$id --conf cluster.conf > $(LOG_DIR)/node_$$id.log 2>&1 < /dev/null &"; \
	done
	@echo "All start commands initiated."

# 5. Raftの停止
kill:
	@for id in $(IDS); do \
	   ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
	   bin="$(BINARY_NAME)_$$id"; \
	   echo "[$$ip] Killing $$bin..."; \
	   ssh $(USER)@$$ip "pkill -x $$bin || echo 'Not running.'" & \
	done; wait

# 6. バイナリの削除
clean:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Cleaning $$bin..."; \
		ssh $(USER)@$$ip "cd $(PROJECT_DIR) && rm $$bin || echo 'Binary not existing'" & \
	done; wait