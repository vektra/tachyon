package tachyon

import (
	"fmt"
	"github.com/jessevdk/go-flags"
)

type Options struct {
	Vars       map[string]string `short:"s" long:"set" description:"Set a variable"`
	ShowOutput bool              `short:"o" long:"output" description:"Show command output"`
	Host       string            `short:"h" long:"host" description:"Run the playbook on another host"`
}

func Main(args []string) int {
	var opts Options

	args, err := flags.ParseArgs(&opts, args)

	if err != nil {
		fmt.Printf("Error parsing options: %s", err)
		return 1
	}

	if len(args) != 2 {
		fmt.Printf("Usage: tachyon [options] <playbook>\n")
		return 1
	}

	if opts.Host != "" {
		return runOnHost(&opts, args)
	}

	cfg := &Config{ShowCommandOutput: opts.ShowOutput}

	ns := NewNestedScope(nil)

	for k, v := range opts.Vars {
		ns.Set(k, v)
	}

	env := NewEnv(ns, cfg)
	defer env.Cleanup()

	playbook, err := NewPlaybook(env, args[1])
	if err != nil {
		fmt.Printf("Error loading plays: %s\n", err)
		return 1
	}

	runner := NewRunner(env, playbook.Plays)
	err = runner.Run(env)

	if err != nil {
		fmt.Printf("Error running playbook: %s\n", err)
		return 1
	}

	return 0
}

func runOnHost(opts *Options, args []string) int {
	return 1
}
