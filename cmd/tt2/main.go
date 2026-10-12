package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tinmancoding/tasktree/v2/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(cli.Code(err))
	}
}
