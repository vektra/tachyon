package tachyon

import (
	"bytes"
	"fmt"
	"github.com/jessevdk/go-flags"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Options struct {
	Vars        map[string]string `short:"s" long:"set" description:"Set a variable"`
	ShowOutput  bool              `short:"o" long:"output" description:"Show command output"`
	Host        string            `short:"t" long:"host" description:"Run the playbook on another host"`
	Development bool              `long:"dev" description:"Use a dev version of tachyon"`
	CleanHost   bool              `long:"clean-host" description:"Clean the host cache before using"`
	Debug       bool              `short:"d" long:"debug" description:"Show all information about commands"`
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

	cur, err := os.Getwd()
	if err != nil {
		fmt.Printf("Unable to figure out the current directory: %s\n", err)
		return 1
	}

	defer os.Chdir(cur)
	os.Chdir(playbook.baseDir)

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

BIN=tachyon-$TACHYON_OS-$TACHYON_ARCH

if ! test -f tachyon; then
  curl -O https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/sums
  if which gpg > /dev/null; then
    gpg --keyserver keys.gnupg.net --recv-key A408199F &
    curl -O https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/sums.asc &
  fi

  curl -O https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/$BIN

  wait

  if which gpg > /dev/null; then
    if ! gpg --verify sums.asc; then
      echo "Signature verification failed! Aborting!"
      exit 1
    fi
  fi

  if ! shasum -c sums; then
    echo "Sum verification failed!"
    exit 1
  fi

  mv $BIN tachyon
  chmod a+x tachyon
fi
`)

func normalizeArch(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	default:
		return arch
	}
}

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

	out, err := ssh.RunAndCapture(bootstrap + " && uname && uname -m")
	if err != nil {
		fmt.Printf("Error creating remote .tachyon dir: %s\n", err)
		return 1
	}

	tos, arch, ok := split2(string(out), "\n")
	if !ok {
		fmt.Printf("Unable to figure out os and arch of remote machine\n")
		return 1
	}

	tos = strings.ToLower(tos)
	arch = normalizeArch(strings.TrimSpace(arch))

	binary := fmt.Sprintf("tachyon-%s-%s", tos, arch)

	if opts.Development {
		fmt.Printf("=== Copying development tachyon...\n")

		path := filepath.Dir(args[0])

		err = ssh.CopyToHost(filepath.Join(path, binary), ".tachyon/tachyon")
		if err != nil {
			fmt.Printf("Error copying tachyon to vagrant: %s\n", err)
			return 1
		}
	} else {
		fmt.Printf("=== Updating tachyon release...\n")

		c := ssh.Command("cat > .tachyon/update && chmod a+x .tachyon/update")

		c.Stdout = os.Stdout
		c.Stdin = bytes.NewReader(cUpdateScript)
		err = c.Run()
		if err != nil {
			fmt.Printf("Error updating, well, the updater: %s\n", err)
			return 1
		}

		cmd := fmt.Sprintf("TACHYON_OS=%s TACHYON_ARCH=%s ./.tachyon/update", tos, arch)
		err = ssh.Run(cmd)
		if err != nil {
			fmt.Printf("Error running updater: %s\n", err)
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
