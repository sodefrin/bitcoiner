package subcmd

import (
	"context"
	"errors"

	"github.com/sodefrin/bitcoiner/config"
	"github.com/sodefrin/bitcoiner/logger"
	"github.com/sodefrin/bitcoiner/marketmake"
	"github.com/sodefrin/bitcoiner/sma"
	"github.com/sodefrin/bitcoiner/trace"
	"github.com/sodefrin/bitflyer"
	"golang.org/x/sync/errgroup"
)

type Cmd interface {
	Execute(args []string) error
}

var (
	ErrUnknownCommand = errors.New("usage: bitcoiner <subcmd>")
)

type Subcmd struct {
	ProjectID      string `envconfig:"PROJECT_ID" required:"true"`
	BitflyerKey    string `envconfig:"BITFLYER_KEY" required:"true"`
	BitflyerSecret string `envconfig:"BITFLYER_SECRET" required:"true"`
}

func NewSubcmd() (*Subcmd, error) {
	cmds := &Subcmd{}

	if err := config.GetEnv(cmds); err != nil {
		return nil, err
	}

	return cmds, nil
}

func (s *Subcmd) Execute(args []string) error {
	if len(args) < 2 {
		return ErrUnknownCommand
	}

	l, err := logger.NewLogger(s.ProjectID, "bitcoiner")
	if err != nil {
		return err
	}

	bf := bitflyer.NewBitflyer()

	realtime, err := bf.GetRealtimeAPIClient()
	if err != nil {
		return err
	}

	private, err := bf.PrivateAPIClient(s.BitflyerKey, s.BitflyerSecret)
	if err != nil {
		return err
	}

	var cmd Cmd
	switch args[1] {
	case "marketmake":
		mm, err := marketmake.NewMarketMake(l, realtime, private)
		if err != nil {
			return err
		}
		cmd = mm
	case "sma":
		sma, err := sma.NewSma(l, realtime, private)
		if err != nil {
			return err
		}
		cmd = sma
	}

	ctx := context.Background()
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return cmd.Execute(args[:1])
	})
	eg.Go(func() error {
		return trace.Trace(ctx, l, private, s.ProjectID)
	})

	return eg.Wait()
}
