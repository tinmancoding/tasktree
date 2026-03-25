package main

import (
	"os"

	"github.com/tinmancoding/tasktree/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
