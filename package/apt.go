package apt

import (
	"fmt"
	"github.com/vektra/tachyon"
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

	rd := tachyon.ResultData{
		"installed": curVer,
		"candidate": canVer,
	}

	if curVer == canVer {
		return tachyon.WrapResult(false, rd), nil
	}

	if a.Dry {
		rd["dryrun"] = true
		return tachyon.WrapResult(true, rd), nil
	}

	return nil, fmt.Errorf("not yet")
}

func init() {
	tachyon.RegisterCommand("apt", &Apt{})
}
