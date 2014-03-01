package apt

import (
	"fmt"
	"github.com/vektra/tachyon"
	"os"
	"os/exec"
	"regexp"
)

type Apt struct {
	Pkg   string `tachyon:"pkg,required"`
	State string `tachyon:"state"`
	Dry   bool   `tachyon:"dryrun"`
}

var installed = regexp.MustCompile(`Installed: ([^\n]+)`)
var candidate = regexp.MustCompile(`Candidate: ([^\n]+)`)

func (a *Apt) Run(env *tachyon.Environment, args string) (*tachyon.Result, error) {
	state := a.State
	if state == "" {
		state = "present"
	}

	out, err := exec.Command("apt-cache", "policy", a.Pkg).CombinedOutput()
	if err != nil {
		return nil, err
	}

	res := installed.FindSubmatch(out)
	if res == nil {
		return nil, fmt.Errorf("No package '%s' available")
	}

	curVer := string(res[1])
	if curVer == "(none)" {
		curVer = ""
	}

	res = candidate.FindSubmatch(out)
	if res == nil {
		return nil, fmt.Errorf("Error parsing apt-cache output")
	}

	canVer := string(res[1])

	if state == "absent" {
		rd := tachyon.ResultData{}

		if curVer == "" {
			return tachyon.WrapResult(false, rd), nil
		}

		rd.Set("removed", curVer)

		c := exec.Command("apt-get", "remove", "-y", a.Pkg)
		out, err = c.CombinedOutput()

		if err != nil {
			return nil, err
		}

		return tachyon.WrapResult(true, rd), nil
	}

	rd := tachyon.ResultData{}
	rd.Set("installed", curVer)
	rd.Set("candidate", canVer)

	if state == "present" && curVer == canVer {
		return tachyon.WrapResult(false, rd), nil
	}

	if a.Dry {
		rd.Set("dryrun", true)
		return tachyon.WrapResult(true, rd), nil
	}

	e := append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "DEBIAN_PRIORITY=critical")

	c := exec.Command("apt-get", "install", "-y", a.Pkg)
	c.Env = e
	out, err = c.CombinedOutput()
	if err != nil {
		return nil, err
	}

	rd.Set("installed", canVer)

	return tachyon.WrapResult(true, rd), nil
}

func init() {
	tachyon.RegisterCommand("apt", &Apt{})
}
