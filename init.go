package main

import (
	"os"

	"github.com/TKTHdev/tsujido"
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
					clientAddr := parseClientAddr(conf, id)
					r := NewRaft(id, conf, writeBatchSize, readBatchSize, debug, asyncLog, clientAddr)
					r.Run()
					return nil
				},
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "id",
						Usage: "Node ID",
					},
					&cli.StringFlag{
						Name:  "conf",
						Usage: "Path to config file",
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
				},
			},
			{
				Name:  "client",
				Usage: "Start the YCSB benchmark client",
				Action: func(c *cli.Context) error {
					writeAddr := c.String("write-addr")
					readAddr := c.String("read-addr")
					if readAddr == "" {
						readAddr = writeAddr
					}
					workers := c.Int("workers")
					workloadStr := c.String("workload")
					workload := 50
					switch workloadStr {
					case "ycsb-a":
						workload = 50
					case "ycsb-b":
						workload = 5
					case "ycsb-c":
						workload = 0
					}

					client, err := tsujido.NewTCPClient(writeAddr, readAddr)
					if err != nil {
						return err
					}
					defer client.Close()

					runner := tsujido.NewYCSBRunner(client, tsujido.YCSBConfig{
						Workers:  workers,
						Workload: workload,
						Protocol: "raft",
					})
					runner.Run()
					return nil
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "write-addr",
						Usage:    "Address of the leader node's client port (for writes)",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "read-addr",
						Usage: "Address for reads (defaults to write-addr)",
					},
					&cli.IntFlag{
						Name:  "workers",
						Usage: "Number of concurrent clients",
						Value: 256,
					},
					&cli.StringFlag{
						Name:  "workload",
						Usage: "Workload type (ycsb-a, ycsb-b, ycsb-c)",
						Value: "ycsb-a",
					},
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
