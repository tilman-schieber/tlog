package main

import (
	"os"

	"github.com/tilman-schieber/tlog/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
