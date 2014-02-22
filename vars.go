package tachyon

import (
	"strconv"
)

type Vars map[string]interface{}

func (v Vars) Copy() Vars {
	o := make(Vars)

	for k, v := range v {
		o[k] = v
	}

	return o
}

func VarsFromStrMap(sm map[string]string) Vars {
	o := make(Vars)

	for k, v := range sm {
		if i, err := strconv.ParseInt(k, 0, 0); err != nil {
			o[k] = i
		} else {
			o[k] = v
		}
	}

	return o
}
