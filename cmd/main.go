package main

import (
	"os"

	"raft"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "raft",
		Usage: "A simple Raft implementation",
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start the Raft node",
				Action: func(c *cli.Context) error {
					id := c.Int("id")
					conf := c.String("conf")
					writeBatchSize := c.Int("write-batch-size")
					readBatchSize := c.Int("read-batch-size")
					debug := c.Bool("debug")
					asyncLog := c.Bool("async-log")
					storageType := c.String("storage")
					r := raft.New(raft.Config{
						ID:             id,
						ConfPath:       conf,
						WriteBatchSize: writeBatchSize,
						ReadBatchSize:  readBatchSize,
						Debug:          debug,
						AsyncLog:       asyncLog,
						StorageType:    storageType,
					}, raft.NewKVStore())
					r.Run()
					return nil
				},
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:     "id",
						Usage:    "Node ID",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "conf",
						Usage: "Path to config file",
						Value: "cluster.conf",
					},
					&cli.IntFlag{
						Name:  "write-batch-size",
						Usage: "Raft disk write batch size",
						Value: 128,
					},
					&cli.IntFlag{
						Name:  "read-batch-size",
						Usage: "Raft read batch size",
						Value: 128,
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Enable debug logging",
						Value: false,
					},
					&cli.BoolFlag{
						Name:  "async-log",
						Usage: "Enable asynchronous disk writes",
						Value: false,
					},
					&cli.StringFlag{
						Name:  "storage",
						Usage: "Storage backend: \"file\", \"bitcask\", or \"iouring\"",
						Value: "file",
					},
				},
			},
			{
				Name:  "client",
				Usage: "Run the benchmark client",
				Action: func(c *cli.Context) error {
					conf := c.String("conf")
					workers := c.Int("workers")
					numKeys := c.Int("keys")
					debug := c.Bool("debug")
					workload := 50
					switch c.String("workload") {
					case "ycsb-a":
						workload = 50
					case "ycsb-b":
						workload = 5
					case "ycsb-c":
						workload = 0
					}
					client := NewClient(conf, workers, numKeys, workload, debug)
					client.Run()
					return nil
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "conf",
						Usage: "Path to config file",
						Value: "cluster.conf",
					},
					&cli.IntFlag{
						Name:  "workers",
						Usage: "Number of concurrent workers",
						Value: 1,
					},
					&cli.StringFlag{
						Name:  "workload",
						Usage: "Workload type (ycsb-a, ycsb-b, ycsb-c)",
						Value: "ycsb-a",
					},
					&cli.IntFlag{
						Name:  "keys",
						Usage: "Number of keys to use in benchmark",
						Value: 6,
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Enable debug logging",
						Value: false,
					},
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
