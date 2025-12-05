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
					r := NewRaft(id, conf)
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
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
