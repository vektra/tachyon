package tachyon

import (
	"bytes"
	"fmt"
	"github.com/jessevdk/go-flags"
	"os"
	"os/exec"
	"path/filepath"
)

type Options struct {
	Vars       map[string]string `short:"s" long:"set" description:"Set a variable"`
	ShowOutput bool              `short:"o" long:"output" description:"Show command output"`
	Host       string            `short:"t" long:"host" description:"Run the playbook on another host"`
	Release    bool              `short:"r" long:"release" description:"Use a release version"`
	CleanHost  bool              `long:"clean-host" description:"Clean the host cache before using"`
	Debug      bool              `short:"d" long:"debug" description:"Show all information about commands"`
}

func Main(args []string) int {
	var opts Options

	args, err := flags.ParseArgs(&opts, args)

	if err != nil {
		if serr, ok := err.(*flags.Error); ok {
			if serr.Type == flags.ErrHelp {
				return 2
			}
		}

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

var cUpdateScript = []byte(`#!/bin/bash

cd .tachyon

if ! test -f tachyon; then
  curl -O https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/sums
  if which gpg > /dev/null; then
    gpg --keyserver keys.gnupg.net --recv-key A408199F &
    curl -O https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/sums.asc &
  fi

  curl -O https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/tachyon-linux-amd64

  wait

  if which gpg > /dev/null; then
    if ! gpg --verify sums.asc; then
      echo "Signature verification failed! Aborting!"
      exit 1
    fi
  fi

  mv tachyon-linux-amd64 tachyon
  chmod a+x tachyon
fi
`)

func runOnHost(opts *Options, args []string) int {
	fmt.Printf("=== Executing playbook on %s\n", opts.Host)

	ssh := NewSSH(opts.Host)
	ssh.Debug = opts.Debug

	defer ssh.Cleanup()

	var bootstrap string

	if opts.CleanHost {
		bootstrap = "rm -rf .tachyon && mkdir -p .tachyon"
	} else {
		bootstrap = "mkdir -p .tachyon"
	}

	err := ssh.Run(bootstrap)
	if err != nil {
		fmt.Printf("Error creating remote .tachyon dir: %s\n", err)
		return 1
	}

	if opts.Release {
		fmt.Printf("=== Updating tachyon release...\n")

		c := ssh.Command("cat > .tachyon/update && chmod a+x .tachyon/update")

		c.Stdout = os.Stdout
		c.Stdin = bytes.NewReader(cUpdateScript)
		err = c.Run()
		if err != nil {
			fmt.Printf("Error updating, well, the updater: %s\n", err)
			return 1
		}

		err = ssh.Run("./.tachyon/update")
		if err != nil {
			fmt.Printf("Error running updater: %s\n", err)
			return 1
		}
	} else {
		err = ssh.CopyToHost("tachyon-linux-amd64", ":.tachyon/tachyon")
		if err != nil {
			fmt.Printf("Error copying tachyon to vagrant: %s\n", err)
			return 1
		}
	}

	var src string

	var main string

	fi, err := os.Stat(args[1])
	if fi.IsDir() {
		src, err = filepath.Abs(args[1])
		if err != nil {
			fmt.Printf("Unable to resolve %s: %s", args[1], err)
		}
		main = "site.yml"
	} else {
		abs, err := filepath.Abs(args[1])
		if err != nil {
			fmt.Printf("Unable to resolve %s: %s", args[1], err)
			return 1
		}

		main = filepath.Base(abs)
		src = filepath.Dir(abs)
	}

	src += "/"

	fmt.Printf("=== Syncing playbook...\n")

	c := exec.Command("rsync", "-av", "-e", ssh.RsyncCommand(), src, ssh.Host+":.tachyon/playbook")

	if opts.Debug {
		c.Stdout = os.Stdout
	}

	err = c.Run()

	if err != nil {
		fmt.Printf("Error copying playbook to vagrant: %s\n", err)
	}

	fmt.Printf("=== Running playbook...\n")
	startCmd := fmt.Sprintf("cd .tachyon && sudo ./tachyon -o playbook/%s", main)
	err = ssh.RunAndShow(startCmd)

	if err != nil {
		fmt.Printf("Error running playbook on vagrant: %s\n", err)
		return 1
	}

	return 0
}
