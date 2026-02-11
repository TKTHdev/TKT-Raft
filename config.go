package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type Node struct {
	ID         int    `json:"id"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	ClientPort int    `json:"client_port"`
}

func loadNodes(confPath string) []Node {
	file, err := os.ReadFile(confPath)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var nodes []Node
	if err := json.Unmarshal(file, &nodes); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}
	return nodes
}

func parseConfig(confPath string) map[int]string {
	nodes := loadNodes(confPath)

	peerIPs := make(map[int]string)
	for _, node := range nodes {
		peerIPs[node.ID] = fmt.Sprintf("%s:%d", node.IP, node.Port)
	}
	return peerIPs
}

func parseClientAddr(confPath string, nodeID int) string {
	nodes := loadNodes(confPath)
	for _, node := range nodes {
		if node.ID == nodeID {
			return fmt.Sprintf("%s:%d", node.IP, node.ClientPort)
		}
	}
	log.Fatalf("Node %d not found in config", nodeID)
	return ""
}
