package tachyon

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/flynn/go-shlex"
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
		if e2, ok := err.(*exec.ExitError); ok {
			if s, ok := e2.Sys().(syscall.WaitStatus); ok {
				rc = s.ExitStatus()
			} else {
				return nil, fmt.Errorf("Unimplemented for system where exec.ExitError.Sys() is not syscall.WaitStatus.,err:%s", err)
			}
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
		if e2, ok := err.(*exec.ExitError); ok {
			if s, ok := e2.Sys().(syscall.WaitStatus); ok {
				rc = s.ExitStatus()
			} else {
				return nil, fmt.Errorf("Unimplemented for system where exec.ExitError.Sys() is not syscall.WaitStatus.,err:%s", err)
			}
		} else {
			return nil, err
		}
	}

	return &CommandResult{rc, stdout, stderr}, nil
}

type runCmdParam struct {
	IgnoreFail    bool
	IgnoreChanged bool
	ManualStatus  bool
	ChangedRc     int
	OkRc          int
	ChangedCreate string
}

func runCmd(env *CommandEnv, cmd runCmdParam, parts ...string) (*Result, error) {
	res, err := RunCommand(env, parts...)
	if res == nil && err != nil {
		return FailureResult(err), err
	}
	r := NewResult(!cmd.IgnoreChanged)
	r.Add("rc", res.ReturnCode)
	r.Add("stdout", strings.TrimSpace(string(res.Stdout)))
	r.Add("stderr", strings.TrimSpace(string(res.Stderr)))

	if str, ok := renderShellResult(r); ok {
		r.Add("_result", str)
	}

	if cmd.ManualStatus {
		switch {
		case cmd.ChangedRc == res.ReturnCode:
			r.Changed = true
		case cmd.OkRc == res.ReturnCode:
			r.Changed = false
		default:
			return FailureResult(err), err
		}
		return r, nil
	}
	if !cmd.IgnoreFail && err != nil {
		return nil, err
	}
	r.Changed = !cmd.IgnoreChanged
	if res.ReturnCode != 0 {
		r.Failed = true
		if !cmd.IgnoreFail {
			return r, fmt.Errorf("Return code:%d, stderr:%s", res.ReturnCode, res.Stderr)
		}
	} else if r.Changed && cmd.ChangedCreate != "" {
		cf, err := os.Create(cmd.ChangedCreate)
		if err != nil {
			return FailureResult(err), err
		}
		defer cf.Close()
		if _, err = fmt.Fprintf(cf, "%s result:%# v", time.Now(), r); err != nil {
			return FailureResult(err), err
		}
	}
	return r, nil
}

type CommandCmd struct {
	Command       string `tachyon:"command,required"`
	Creates       string `tachyon:"creates"`
	IgnoreFail    bool   `tachyon:"ignore_failure"`
	IgnoreChanged bool   `tachyon:"ignore_changed"`
	ManualStatus  bool   `tachyon:"manual_status"`
	OkRc          int    `tachyon:"ok_rc"`
	ChangedRc     int    `tachyon:"changed_rc"`
	ChangedCreate string `tachyon:"changed_create"`
}

func (cmd *CommandCmd) Run(env *CommandEnv) (*Result, error) {
	if cmd.Creates != "" {
		if _, err := os.Stat(cmd.Creates); err == nil {
			r := NewResult(false)
			r.Add("rc", 0)
			r.Add("exists", cmd.Creates)

			return r, nil
		}
	}

	parts, err := shlex.Split(cmd.Command)

	if err != nil {
		return FailureResult(err), err
	}

	param := runCmdParam{
		IgnoreFail:    cmd.IgnoreFail,
		IgnoreChanged: cmd.IgnoreChanged,
		ManualStatus:  cmd.ManualStatus,
		ChangedRc:     cmd.ChangedRc,
		OkRc:          cmd.OkRc,
		ChangedCreate: cmd.ChangedCreate,
	}
	return runCmd(env, param, parts...)
}

func (cmd *CommandCmd) ParseArgs(s Scope, args string) (Vars, error) {
	if args == "" {
		return Vars{}, nil
	}

	return Vars{"command": Any(args)}, nil
}

type ShellCmd struct {
	Command       string `tachyon:"command,required"`
	Creates       string `tachyon:"creates"`
	IgnoreFail    bool   `tachyon:"ignore_failure"`
	IgnoreChanged bool   `tachyon:"ignore_changed"`
	ManualStatus  bool   `tachyon:"manual_status"`
	OkRc          int    `tachyon:"ok_rc"`
	ChangedRc     int    `tachyon:"changed_rc"`
	ChangedCreate string `tachyon:"changed_create"`
}

func (cmd *ShellCmd) Run(env *CommandEnv) (*Result, error) {
	if cmd.Creates != "" {
		if _, err := os.Stat(cmd.Creates); err == nil {
			r := NewResult(false)
			r.Add("rc", 0)
			r.Add("exists", cmd.Creates)

			return r, nil
		}
	}

	param := runCmdParam{
		IgnoreFail:    cmd.IgnoreFail,
		IgnoreChanged: cmd.IgnoreChanged,
		ManualStatus:  cmd.ManualStatus,
		ChangedRc:     cmd.ChangedRc,
		OkRc:          cmd.OkRc,
		ChangedCreate: cmd.ChangedCreate,
	}
	return runCmd(env, param, "sh", "-c", cmd.Command)
}

func (cmd *ShellCmd) ParseArgs(s Scope, args string) (Vars, error) {
	if args == "" {
		return Vars{}, nil
	}

	return Vars{"command": Any(args)}, nil
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
		return "OK", true
	} else if len(stderr) == 0 && len(stdout) < 60 {
		stdout = strings.Replace(stdout, "\n", " ", -1)
		return fmt.Sprintf(`rc: %d, stdout: "%s"`, rc, stdout), true
	}

	return "", false
}

func md5string(s string) []byte {
	h := md5.New()
	io.WriteString(h, s)
	return h.Sum(nil)
}
func md5file(path string) ([]byte, error) {
	h := md5.New()

	i, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer i.Close()

	if _, err := io.Copy(h, i); err != nil {
		return nil, err
	}

	return h.Sum(nil), nil
}

type MkFileCmd struct {
	Src   string `tachyon:"src,required"`
	Dest  string `tachyon:"dest,required"`
	Owner string `tachyon:"owner"`
	Uid   int    `tachyon:"uid"`
	Gid   int    `tachyon:"gid"`
	Mode  int    `tachyon:"mode"`
}

func (cmd *MkFileCmd) Run(env *CommandEnv) (*Result, error) {
	srcDigest := md5string(cmd.Src)
	var dstDigest []byte

	dest := cmd.Dest

	link := false

	destStat, err := os.Lstat(dest)
	if err == nil {
		dstDigest, _ = md5file(dest)
		link = destStat.Mode()&os.ModeSymlink != 0
	}

	rd := ResultData{
		"md5sum": Any(hex.EncodeToString(srcDigest)),
		"src":    Any(cmd.Src),
		"dest":   Any(dest),
	}

	if dstDigest != nil && bytes.Equal(srcDigest, dstDigest) {
		changed := false

		if cmd.Mode != 0 && destStat.Mode() != os.FileMode(cmd.Mode) {
			changed = true
			if err := os.Chmod(dest, os.FileMode(cmd.Mode)); err != nil {
				return FailureResult(err), err
			}
		}
		if cmd.Uid, cmd.Gid, err = ChangePerm(cmd.Owner, cmd.Uid, cmd.Gid); err != nil {
			return FailureResult(err), err
		}
		if estat, ok := destStat.Sys().(*syscall.Stat_t); ok {
			if cmd.Uid != int(estat.Uid) || cmd.Gid != int(estat.Gid) {
				changed = true
				os.Chown(dest, cmd.Uid, cmd.Gid)
			}
		}

		return WrapResult(changed, rd), nil
	}

	tmp := fmt.Sprintf("%s.tmp.%d", cmd.Dest, os.Getpid())

	output, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return FailureResult(err), err
	}

	defer output.Close()

	if _, err = output.Write([]byte(cmd.Src)); err != nil {
		os.Remove(tmp)
		return FailureResult(err), err
	}

	if link {
		os.Remove(dest)
	}

	if cmd.Mode != 0 {
		if err := os.Chmod(tmp, os.FileMode(cmd.Mode)); err != nil {
			os.Remove(tmp)
			return FailureResult(err), err
		}
	}

	if cmd.Mode != 0 {
		if cmd.Uid, cmd.Gid, err = ChangePerm(cmd.Owner, cmd.Uid, cmd.Gid); err != nil {
			return FailureResult(err), err
		}
	}
	os.Chown(tmp, cmd.Uid, cmd.Gid)

	err = os.Rename(tmp, dest)
	if err != nil {
		os.Remove(tmp)
		return FailureResult(err), err
	}

	return WrapResult(true, rd), nil
}

type CopyCmd struct {
	Src   string `tachyon:"src,required"`
	Dest  string `tachyon:"dest,required"`
	Owner string `tachyon:"owner"`
	Uid   int    `tachyon:"uid"`
	Gid   int    `tachyon:"gid"`
	Mode  int    `tachyon:"mode"`
}

func (cmd *CopyCmd) Run(env *CommandEnv) (*Result, error) {
	var src string

	if cmd.Src[0] == '/' {
		src = cmd.Src
	} else {
		src = env.Paths.File(cmd.Src)
	}

	if cmd.Mode == 0 {
		fi, err := os.Stat(src)
		if err != nil {
			return FailureResult(err), err
		}
		cmd.Mode = int(fi.Mode())

	}
	input, err := os.Open(src)

	if err != nil {
		return FailureResult(err), err
	}
	defer input.Close()

	srcDigest, err := md5file(src)
	if err != nil {
		return FailureResult(err), err
	}

	var dstDigest []byte

	dest := cmd.Dest

	link := false

	destStat, err := os.Lstat(dest)
	if err == nil {
		if destStat.IsDir() {
			dest = filepath.Join(dest, filepath.Base(src))
		} else {
			dstDigest, _ = md5file(dest)
		}

		link = destStat.Mode()&os.ModeSymlink != 0
	}

	rd := ResultData{
		"md5sum": Any(hex.EncodeToString(srcDigest)),
		"src":    Any(src),
		"dest":   Any(dest),
	}

	if dstDigest != nil && bytes.Equal(srcDigest, dstDigest) {
		changed := false

		if cmd.Mode != 0 && destStat.Mode() != os.FileMode(cmd.Mode) {
			changed = true
			if err := os.Chmod(dest, os.FileMode(cmd.Mode)); err != nil {
				return FailureResult(err), err
			}
		}

		if cmd.Uid, cmd.Gid, err = ChangePerm(cmd.Owner, cmd.Uid, cmd.Gid); err != nil {
			return FailureResult(err), err
		}
		if estat, ok := destStat.Sys().(*syscall.Stat_t); ok {
			if cmd.Uid != int(estat.Uid) || cmd.Gid != int(estat.Gid) {
				changed = true
				os.Chown(dest, cmd.Uid, cmd.Gid)
			}
		}

		return WrapResult(changed, rd), nil
	}

	tmp := fmt.Sprintf("%s.tmp.%d", cmd.Dest, os.Getpid())

	output, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return FailureResult(err), err
	}

	defer output.Close()

	if _, err = io.Copy(output, input); err != nil {
		os.Remove(tmp)
		return FailureResult(err), err
	}

	if link {
		os.Remove(dest)
	}

	if err := os.Chmod(tmp, os.FileMode(cmd.Mode)); err != nil {
		os.Remove(tmp)
		return FailureResult(err), err
	}

	if cmd.Uid, cmd.Gid, err = ChangePerm(cmd.Owner, cmd.Uid, cmd.Gid); err != nil {
		return FailureResult(err), err
	}
	os.Chown(tmp, cmd.Uid, cmd.Gid)

	err = os.Rename(tmp, dest)
	if err != nil {
		os.Remove(tmp)
		return FailureResult(err), err
	}

	return WrapResult(true, rd), nil
}

type ScriptCmd struct {
	Script        string `tachyon:"command,required"`
	Creates       string `tachyon:"creates"`
	IgnoreFail    bool   `tachyon:"ignore_failure"`
	IgnoreChanged bool   `tachyon:"ignore_changed"`
	ManualStatus  bool   `tachyon:"manual_status"`
	OkRc          int    `tachyon:"ok_rc"`
	ChangedRc     int    `tachyon:"changed_rc"`
	ChangedCreate string `tachyon:"changed_create"`
}

func (cmd *ScriptCmd) ParseArgs(s Scope, args string) (Vars, error) {
	if args == "" {
		return Vars{}, nil
	}

	return Vars{"command": Any(args)}, nil
}

func (cmd *ScriptCmd) Run(env *CommandEnv) (*Result, error) {
	if cmd.Creates != "" {
		if _, err := os.Stat(cmd.Creates); err == nil {
			r := NewResult(false)
			r.Add("rc", 0)
			r.Add("exists", cmd.Creates)

			return r, nil
		}
	}

	script := cmd.Script

	parts, err := shlex.Split(cmd.Script)
	if err == nil {
		script = parts[0]
	}

	path := env.Paths.File(script)

	_, err = os.Stat(path)
	if err != nil {
		return FailureResult(err), err
	}

	runArgs := append([]string{"sh", path}, parts[1:]...)

	param := runCmdParam{
		IgnoreFail:    cmd.IgnoreFail,
		IgnoreChanged: cmd.IgnoreChanged,
		ManualStatus:  cmd.ManualStatus,
		ChangedRc:     cmd.ChangedRc,
		OkRc:          cmd.OkRc,
		ChangedCreate: cmd.ChangedCreate,
	}
	return runCmd(env, param, runArgs...)
}

func init() {
	RegisterCommand("command", &CommandCmd{})
	RegisterCommand("shell", &ShellCmd{})
	RegisterCommand("copy", &CopyCmd{})
	RegisterCommand("mkfile", &MkFileCmd{})
	RegisterCommand("script", &ScriptCmd{})
}

func ChangePerm(owner string, suid, sgid int) (uid, gid int, err error) {
	var u *user.User
	switch {
	case owner != "" && suid != 0:
		err = fmt.Errorf("both uid and owner is specified. owner:%s,uid:%d", owner, suid)
		return
	case owner == "" && suid == 0 && sgid == 0:
		if u, err = user.Current(); err != nil {
			return
		}
		uid, _ = strconv.Atoi(u.Uid)
		gid, _ = strconv.Atoi(u.Gid)
	case owner != "":
		if u, err = user.Lookup(owner); err != nil {
			return
		}
		uid, _ = strconv.Atoi(u.Uid)
		if sgid != 0 {
			gid = sgid
		} else {
			gid, _ = strconv.Atoi(u.Gid)
		}
	default:
		uid = suid
		gid = sgid
	}
	return
}
