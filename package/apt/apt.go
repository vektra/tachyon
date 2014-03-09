package apt

import (
	"fmt"
	"github.com/vektra/tachyon"
	"os"
	"regexp"
)

type Apt struct {
	Pkg   string `tachyon:"pkg"`
	State string `tachyon:"state"`
	Cache string `tachyon:"cache"`
	Dry   bool   `tachyon:"dryrun"`
}

var installed = regexp.MustCompile(`Installed: ([^\n]+)`)
var candidate = regexp.MustCompile(`Candidate: ([^\n]+)`)

func (a *Apt) Run(env *tachyon.CommandEnv, args string) (*tachyon.Result, error) {
	state := a.State
	if state == "" {
		state = "present"
	}

	if a.Cache == "update" {
		_, err := tachyon.RunCommand(env, "apt-get", "update")
		if err != nil {
			return nil, err
		}
	}

	if a.Pkg == "" {
		simp := tachyon.NewResult(true)
		simp.Add("cache", "updated")

		return simp, nil
	}

	out, err := tachyon.RunCommand(env, "apt-cache", "policy", a.Pkg)
	if err != nil {
		return nil, err
	}

	res := installed.FindSubmatch(out.Stdout)
	if res == nil {
		return nil, fmt.Errorf("No package '%s' available")
	}

	curVer := string(res[1])
	if curVer == "(none)" {
		curVer = ""
	}

	res = candidate.FindSubmatch(out.Stdout)
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

		_, err = tachyon.RunCommand(env, "apt-get", "remove", "-y", a.Pkg)

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

	_, err = tachyon.RunCommandInEnv(env, e, "apt-get", "install", "-y", a.Pkg)
	if err != nil {
		return nil, err
	}

	rd.Set("installed", canVer)

	return tachyon.WrapResult(true, rd), nil
}

func init() {
	tachyon.RegisterCommand("apt", &Apt{})
}
