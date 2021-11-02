package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/lthibault/log"
	"github.com/urfave/cli/v2"
)

const version = "0.0.0"

var flags = []cli.Flag{
	&cli.StringFlag{
		Name:    "logfmt",
		Aliases: []string{"f"},
		Usage:   "text, json, none",
		Value:   "text",
		EnvVars: []string{"WW_LOGFMT"},
	},
	&cli.StringFlag{
		Name:    "loglvl",
		Usage:   "trace, debug, info, warn, error, fatal",
		Value:   "info",
		EnvVars: []string{"WW_LOGLVL"},
	},
	&cli.BoolFlag{
		Name:    "prettyprint",
		Aliases: []string{"pp"},
		Usage:   "pretty-print JSON output",
		Hidden:  true,
	},
}

var commands = []*cli.Command{
	// discover.Command(),
	// start.Command(),
}

func main() {
	run(&cli.App{
		Name:                 "CASM",
		Usage:                "Cluster Assembly",
		UsageText:            "casm [global options] command [command options] [arguments...]",
		Copyright:            "2020 The Wetware Project",
		Version:              version,
		EnableBashCompletion: true,
		Flags:                flags,
		Commands:             commands,
		Before:               before(),
	})
}

func before() cli.BeforeFunc {
	// We attempt to gracefully exit the application when we receive certain
	// signals from the OS.  Throughout the application, we guarantee that
	// c.Context will expire when the local process receives SIGINT or SIGTERM.
	return func(c *cli.Context) error {
		var cancel context.CancelFunc
		c.Context, cancel = context.WithCancel(c.Context)

		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			defer cancel()
			<-ch
		}()

		return nil
	}
}

func run(app *cli.App) {
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
