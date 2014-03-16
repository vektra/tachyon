package tachyon

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/flynn/go-shlex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

func captureCmd(c *exec.Cmd, show bool) ([]byte, []byte, error) {
	stdout, err := c.StdoutPipe()

	if err != nil {
		return nil, nil, err
	}

	defer stdout.Close()

	var wg sync.WaitGroup

	var bout bytes.Buffer
	var berr bytes.Buffer

	prefix := []byte(`| `)

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := bufio.NewReader(stdout)

		for {
			line, err := buf.ReadSlice('\n')

			if err != nil {
				break
			}

			bout.Write(line)

			if show {
				os.Stdout.Write(prefix)
				os.Stdout.Write(line)
			}
		}
	}()

	stderr, err := c.StderrPipe()

	if err != nil {
		stdout.Close()
		return nil, nil, err
	}

	defer stderr.Close()

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := bufio.NewReader(stderr)

		for {
			line, err := buf.ReadSlice('\n')

			if err != nil {
				break
			}

			berr.Write(line)

			if show {
				os.Stdout.Write(prefix)
				os.Stdout.Write(line)
			}
		}
	}()

	c.Start()

	wg.Wait()

	err = c.Wait()

	return bout.Bytes(), berr.Bytes(), err
}

type CommandResult struct {
	ReturnCode int
	Stdout     []byte
	Stderr     []byte
}

func RunCommand(env *CommandEnv, parts ...string) (*CommandResult, error) {
	c := exec.Command(parts[0], parts[1:]...)

	if env.Env.config.ShowCommandOutput {
		fmt.Printf("RUN: %s\n", strings.Join(parts, " "))
	}

	rc := 0

	stdout, stderr, err := captureCmd(c, env.Env.config.ShowCommandOutput)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			rc = 1
		} else {
			return nil, err
		}
	}

	return &CommandResult{rc, stdout, stderr}, nil
}

func RunCommandInEnv(env *CommandEnv, unixEnv []string, parts ...string) (*CommandResult, error) {
	c := exec.Command(parts[0], parts[1:]...)
	c.Env = unixEnv

	if env.Env.config.ShowCommandOutput {
		fmt.Printf("RUN: %s\n", strings.Join(parts, " "))
	}

	rc := 0

	stdout, stderr, err := captureCmd(c, env.Env.config.ShowCommandOutput)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			rc = 1
		} else {
			return nil, err
		}
	}

	return &CommandResult{rc, stdout, stderr}, nil
}

func runCmd(env *CommandEnv, parts ...string) (*Result, error) {
	cmd, err := RunCommand(env, parts...)
	if err != nil {
		return nil, err
	}

	r := NewResult(true)

	r.Add("rc", cmd.ReturnCode)
	r.Add("stdout", strings.TrimSpace(string(cmd.Stdout)))
	r.Add("stderr", strings.TrimSpace(string(cmd.Stderr)))

	if str, ok := renderShellResult(r); ok {
		r.Add("$result", str)
	}

	return r, nil
}

type CommandCmd struct{}

func (cmd *CommandCmd) Run(env *CommandEnv, args string) (*Result, error) {
	parts, err := shlex.Split(args)

	if err != nil {
		return nil, err
	}

	return runCmd(env, parts...)
}

type ShellCmd struct{}

func (cmd *ShellCmd) Run(env *CommandEnv, args string) (*Result, error) {
	return runCmd(env, "sh", "-c", args)
}

func renderShellResult(res *Result) (string, bool) {
	rcv, ok := res.Get("rc")
	if !ok {
		return "", false
	}

	stdoutv, ok := res.Get("stdout")
	if !ok {
		return "", false
	}

	stderrv, ok := res.Get("stderr")
	if !ok {
		return "", false
	}

	rc := rcv.Read().(int)
	stdout := stdoutv.Read().(string)
	stderr := stderrv.Read().(string)

	if rc == 0 && len(stdout) == 0 && len(stderr) == 0 {
		return "", true
	} else if len(stderr) == 0 && len(stdout) < 60 {
		stdout = strings.Replace(stdout, "\n", " ", -1)
		return fmt.Sprintf(`rc: %d, stdout: "%s"`, rc, stdout), true
	}

	return "", false
}

type CopyCmd struct {
	Src  string `tachyon:"src,required"`
	Dest string `tachyon:"dest,required"`
}

func md5file(path string) ([]byte, error) {
	h := md5.New()

	i, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(h, i); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

func (cmd *CopyCmd) Run(env *CommandEnv, args string) (*Result, error) {
	input, err := os.Open(cmd.Src)

	if err != nil {
		return nil, err
	}

	srcStat, err := os.Stat(cmd.Src)
	if err != nil {
		return nil, err
	}

	srcDigest, err := md5file(cmd.Src)
	if err != nil {
		return nil, err
	}

	var dstDigest []byte

	defer input.Close()

	dest := cmd.Dest

	link := false

	if stat, err := os.Lstat(dest); err == nil {
		if stat.IsDir() {
			dest = filepath.Join(dest, filepath.Base(cmd.Src))
		} else {
			dstDigest, _ = md5file(dest)
		}

		link = stat.Mode()&os.ModeSymlink != 0
	}

	rd := ResultData{
		"md5sum": Any(hex.Dump(srcDigest)),
		"src":    Any(cmd.Src),
		"dest":   Any(dest),
	}

	if dstDigest != nil && bytes.Equal(srcDigest, dstDigest) {
		return WrapResult(false, rd), nil
	}

	tmp := fmt.Sprintf("%s.tmp.%d", cmd.Dest, os.Getpid())

	output, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return nil, err
	}

	defer output.Close()

	if _, err = io.Copy(output, input); err != nil {
		os.Remove(tmp)
		return nil, err
	}

	if link {
		os.Remove(dest)
	}

	if err := os.Chmod(tmp, srcStat.Mode()); err != nil {
		os.Remove(tmp)
		return nil, err
	}

	if ostat, ok := srcStat.Sys().(*syscall.Stat_t); ok {
		os.Chown(tmp, int(ostat.Uid), int(ostat.Gid))
	}

	err = os.Rename(tmp, dest)
	if err != nil {
		os.Remove(tmp)
		return nil, err
	}

	return WrapResult(true, rd), nil
}

type ScriptCmd struct{}

func (cmd *ScriptCmd) Run(env *CommandEnv, args string) (*Result, error) {
	script := args

	parts, err := shlex.Split(args)
	if err == nil {
		script = parts[0]
	}

	path := env.Paths.File(script)

	_, err = os.Stat(path)
	if err != nil {
		return nil, err
	}

	runArgs := append([]string{"sh", path}, parts[1:]...)

	return runCmd(env, runArgs...)
}

var cUpdateScript = []byte(`#!/bin/bash

cd .tachyon

REL=$TACHYON_RELEASE
BIN=tachyon-$TACHYON_OS-$TACHYON_ARCH

if test -f tachyon; then
  CUR=$(< release)
  if test "$REL" != "$CUR"; then
    echo "Detected tachyon of old release ($CUR), removing."
    rm tachyon
  fi
fi

if which curl > /dev/null; then
  DL="curl -O"
elif which wget > /dev/null; then
  DL="wget"
else
  echo "No curl or wget, unable to pull a release"
  exit 1
fi

if ! test -f tachyon; then
  echo "Downloading $REL/$BIN..."

  $DL https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/$REL/sums
  if which gpg > /dev/null; then
    gpg --keyserver keys.gnupg.net --recv-key A408199F &
    $DL https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/$REL/sums.asc &
  fi

  $DL https://s3-us-west-2.amazonaws.com/tachyon.vektra.io/$REL/$BIN

  wait

  if which gpg > /dev/null; then
    if ! gpg --verify sums.asc; then
      echo "Signature verification failed! Aborting!"
      exit 1
    fi
  fi

  mv $BIN $BIN.gz

  # If gunzip fails, it's because the file isn't gzip'd, so we
  # assume it's already in the correct format.
  if ! gunzip $BIN.gz; then
    mv $BIN.gz $BIN
  fi

  if which shasum > /dev/null; then
    if ! (grep $BIN sums | shasum -c); then
      echo "Sum verification failed!"
      exit 1
    fi
  else
    echo "No shasum available to verify files"
  fi

  echo $REL > release

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

type Tachyon struct {
	Target   string `tachyon:"target"`
	Debug    bool   `tachyon:"debug"`
	Clean    bool   `tachyon:"clean"`
	Dev      bool   `tachyon:"dev"`
	Playbook string `tachyon:"playbook"`
	Release  string `tachyon:"release"`
}

func (t *Tachyon) Run(env *CommandEnv, args string) (*Result, error) {
	if t.Release == "" {
		t.Release = Release
	}

	ssh := NewSSH(t.Target)
	ssh.Debug = t.Debug

	defer ssh.Cleanup()

	err := ssh.Start()
	if err != nil {
		return nil, fmt.Errorf("Error starting persistent SSH connection: %s\n", err)
	}

	var bootstrap string

	if t.Clean {
		bootstrap = "rm -rf .tachyon && mkdir -p .tachyon"
	} else {
		bootstrap = "mkdir -p .tachyon"
	}

	out, err := ssh.RunAndCapture(bootstrap + " && uname && uname -m")
	if err != nil {
		return nil, fmt.Errorf("Error creating remote .tachyon dir: %s\n", err)
	}

	tos, arch, ok := split2(string(out), "\n")
	if !ok {
		return nil, fmt.Errorf("Unable to figure out os and arch of remote machine\n")
	}

	tos = strings.ToLower(tos)
	arch = normalizeArch(strings.TrimSpace(arch))

	binary := fmt.Sprintf("tachyon-%s-%s", tos, arch)

	if t.Dev {
		env.Progress("Copying development tachyon...")

		path := filepath.Dir(Arg0)

		err = ssh.CopyToHost(filepath.Join(path, binary), ".tachyon/tachyon")
		if err != nil {
			return nil, fmt.Errorf("Error copying tachyon to vagrant: %s\n", err)
		}
	} else {
		env.Progress("Updating tachyon release...")

		c := ssh.Command("cat > .tachyon/update && chmod a+x .tachyon/update")

		c.Stdout = os.Stdout
		c.Stdin = bytes.NewReader(cUpdateScript)
		err = c.Run()
		if err != nil {
			return nil, fmt.Errorf("Error updating, well, the updater: %s\n", err)
		}

		cmd := fmt.Sprintf("TACHYON_RELEASE=%s TACHYON_OS=%s TACHYON_ARCH=%s ./.tachyon/update", t.Release, tos, arch)
		err = ssh.Run(cmd)
		if err != nil {
			return nil, fmt.Errorf("Error running updater: %s\n", err)
		}
	}

	var src string

	var main string

	fi, err := os.Stat(t.Playbook)
	if fi.IsDir() {
		src, err = filepath.Abs(t.Playbook)
		if err != nil {
			return nil, fmt.Errorf("Unable to resolve %s: %s", t.Playbook, err)
		}
		main = "site.yml"
	} else {
		abs, err := filepath.Abs(t.Playbook)
		if err != nil {
			return nil, fmt.Errorf("Unable to resolve %s: %s", t.Playbook, err)
		}

		main = filepath.Base(abs)
		src = filepath.Dir(abs)
	}

	src += "/"

	env.Progress("Syncing playbook...")

	c := exec.Command("rsync", "-av", "-e", ssh.RsyncCommand(), src, ssh.Host+":.tachyon/playbook")

	if t.Debug {
		c.Stdout = os.Stdout
	}

	err = c.Run()

	if err != nil {
		return nil, fmt.Errorf("Error copying playbook to vagrant: %s\n", err)
	}

	env.Progress("Running playbook...")

	startCmd := fmt.Sprintf("cd .tachyon && sudo ./tachyon -o playbook/%s", main)
	err = ssh.RunAndShow(startCmd)

	if err != nil {
		return nil, fmt.Errorf("Error running playbook on vagrant: %s\n", err)
	}

	res := NewResult(true)
	res.Add("target", t.Target)
	res.Add("playbook", t.Playbook)

	return res, nil
}

func init() {
	RegisterCommand("command", &CommandCmd{})
	RegisterCommand("shell", &ShellCmd{})
	RegisterCommand("copy", &CopyCmd{})
	RegisterCommand("script", &ScriptCmd{})
	RegisterCommand("tachyon", &Tachyon{})
}
