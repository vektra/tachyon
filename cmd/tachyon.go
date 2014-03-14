package main

import (
	"github.com/vektra/tachyon"
	_ "github.com/vektra/tachyon/net"
	_ "github.com/vektra/tachyon/package"
	"os"
)

func main() {
	os.Exit(tachyon.Main(os.Args))
}
