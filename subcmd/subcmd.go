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
		return errors.New("usage: bitcoiner <subcmd>")
	}
	cmd := s[args[0]]
	return cmd.Execute(args[:1])
}
