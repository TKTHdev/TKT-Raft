package main

import (
	"os"

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
					batchSize := c.Int("batch-size")
					workers := c.Int("workers")
					debug := c.Bool("debug")
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
					r := NewRaft(id, conf, batchSize, workers, debug, workload)
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
						Name:  "batch-size",
						Usage: "Raft disk batch size",
						Value: 128,
					},
					&cli.IntFlag{
						Name:  "workers",
						Usage: "Number of concurrent clients",
						Value: 256,
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Enable debug logging",
						Value: false,
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
