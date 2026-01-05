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
					r := NewRaft(id, conf, batchSize, workers)
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
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
