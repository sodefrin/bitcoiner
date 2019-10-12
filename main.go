package main

import (
	"log"
	"os"

	"github.com/sodefrin/bitcoiner/subcmd"
)

func main() {
	cmds, err := subcmd.NewSubcmd()
	if err != nil {
		log.Fatal(err)
	}

	if err:=cmds.Execute(os.Args); err != nil {
		log.Fatal(err)
	}
}
