package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
)

type Node struct {
	ID   int    `json:"id"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// ★ 新しく追加・実装する関数
func parseConfig(confPath string) map[int]string {
	file, err := ioutil.ReadFile(confPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var nodes []Node
	if err := json.Unmarshal(file, &nodes); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	peerIPs := make(map[int]string)
	for _, node := range nodes {
		peerIPs[node.ID] = fmt.Sprintf("%s:%d", node.IP, node.Port)
	}
	return peerIPs
}
