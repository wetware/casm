package start

import (
	"context"
	"errors"

	"github.com/urfave/cli/v2"
	bootutil "github.com/wetware/casm/internal/util/boot"
	hostutil "github.com/wetware/casm/internal/util/host"
	pexutil "github.com/wetware/casm/internal/util/pex"
	pubsubutil "github.com/wetware/casm/internal/util/pubsub"
	"github.com/wetware/casm/pkg/cluster"
	"go.uber.org/fx"
)

func Command() *cli.Command {
	return &cli.Command{
		Name:   "start",
		Usage:  "start a CASM host",
		Action: action(),
	}
}

func action() cli.ActionFunc {
	return func(c *cli.Context) error {
		app := fx.New(fx.NopLogger,
			fx.Supply(c),
			fx.Provide(
				bootutil.New,
				hostutil.New,
				pexutil.New,
				pubsubutil.New,
				cluster.New),
			fx.Invoke(run))

		if err := app.Start(c.Context); err != nil {
			return err
		}

		<-app.Done()

		return app.Stop(context.Background())
	}
}

type runtime struct {
	fx.In

	Cluster cluster.Cluster
}

func run(r runtime) error {
	return errors.New("start.go::run() NOT IMPLEMENTED")
}
