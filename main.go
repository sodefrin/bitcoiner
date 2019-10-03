package main

import (
	"fmt"
	"os"

	"github.com/sodefrin/bitcoiner/subcmd"
)

func main() {
	subcmd := subcmd.NewSubcmd()

	if err := subcmd.Execute(os.Args); err != nil {
		fmt.Println(err)
	}
}
