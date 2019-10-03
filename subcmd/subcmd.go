package subcmd

import (
	"errors"

	"github.com/sodefrin/bitcoiner/marketmake"
)

type cmd interface {
	Name() string
	Execute(args []string) error
}

type Subcmd map[string]cmd

var (
	ErrUnknownCommand = errors.New("usage: bitcoiner <subcmd>")
)

func NewSubcmd() Subcmd {
	subcmd := map[string]cmd{}

	add := func(c cmd) {
		subcmd[c.Name()] = c
	}

	add(marketmake.NewMarketMake())

	return subcmd
}

func (s Subcmd) Execute(args []string) error {
	if len(args) < 2 {
		return ErrUnknownCommand
	}

	cmd, ok := s[args[1]]
	if ok {
		return cmd.Execute(args[:1])
	}

	return ErrUnknownCommand
}
