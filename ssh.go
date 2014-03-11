package tachyon

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

type SSH struct {
	Host   string
	Config string
	Debug  bool

	removeConfig bool
	sshOptions   []string
}

func (s *SSH) CommandWithOptions(cmd string, args ...string) []string {
	sshArgs := []string{cmd}
	sshArgs = append(sshArgs, s.sshOptions...)

	if s.Config != "" {
		sshArgs = append(sshArgs, "-F", s.Config)
	}

	return append(sshArgs, args...)
}

func (s *SSH) RsyncCommand() string {
	sshArgs := []string{"ssh"}
	sshArgs = append(sshArgs, s.sshOptions...)

	if s.Config != "" {
		sshArgs = append(sshArgs, "-F", s.Config)
	}

	return strings.Join(sshArgs, " ")
}

func (s *SSH) SSHCommand(cmd string, args ...string) []string {
	sshArgs := []string{cmd}
	sshArgs = append(sshArgs, s.sshOptions...)

	if s.Config != "" {
		sshArgs = append(sshArgs, "-F", s.Config)
	}

	sshArgs = append(sshArgs, s.Host)

	return append(sshArgs, args...)
}

func NewSSH(host string) *SSH {
	s := &SSH{
		Host: host,
	}

	if host == ":vagrant" {
		s.ImportVagrant()
	}

	u, err := user.Current()
	if err != nil {
		panic(err)
	}

	tachDir := u.HomeDir + "/.tachyon"

	if _, err := os.Stat(tachDir); err != nil {
		err = os.Mkdir(tachDir, 0755)
		if err != nil {
			panic(err)
		}
	}

	s.sshOptions = []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=60s",
		"-o", "ControlPath=" + tachDir + "/tachyon-cp-ssh-%h-%p-%r",
	}

	return s
}

func (s *SSH) Cleanup() {
	if s.removeConfig {
		os.Remove(s.Config)
	}
}

func (s *SSH) ImportVagrant() bool {
	s.Host = "default"
	s.removeConfig = true

	out, err := exec.Command("vagrant", "ssh-config").CombinedOutput()
	if err != nil {
		fmt.Printf("Unable to execute 'vagrant ssh-config': %s\n", err)
		return false
	}

	f, err := ioutil.TempFile("", "tachyon")
	if err != nil {
		fmt.Printf("Unable to make tempfile: %s\n", err)
		return false
	}

	_, err = f.Write(out)
	if err != nil {
		fmt.Printf("Unable to write to tempfile: %s\n", err)
		return false
	}

	f.Close()

	s.Config = f.Name()

	return true
}

func (s *SSH) Command(args ...string) *exec.Cmd {
	args = s.SSHCommand("ssh", args...)
	return exec.Command(args[0], args[1:]...)
}

func (s *SSH) Run(args ...string) error {
	c := s.Command(args...)

	if s.Debug {
		fmt.Printf("Run: %#v\n", c.Args)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}

	return c.Run()
}

func (s *SSH) RunAndShow(args ...string) error {
	c := s.Command(args...)

	if s.Debug {
		fmt.Printf("Run: %#v\n", c.Args)
	}

	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	return c.Run()
}

func (s *SSH) CopyToHost(src, dest string) error {
	args := s.CommandWithOptions("scp", src, s.Host+":"+dest)
	c := exec.Command(args[0], args[1:]...)

	if s.Debug {
		fmt.Printf("Run: %#v\n", c.Args)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}

	return c.Run()
}
