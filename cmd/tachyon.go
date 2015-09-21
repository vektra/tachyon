package main

import (
	"os"

	"github.com/vektra/tachyon"
	_ "github.com/vektra/tachyon/net"
	_ "github.com/vektra/tachyon/package"
	_ "github.com/vektra/tachyon/procmgmt"
)

var Release string
var version string

func main() {
	if Release != "" {
		tachyon.Release = Release
	}
	if version != "" {
		tachyon.Version = version
	}

	os.Exit(tachyon.Main(os.Args))
}
