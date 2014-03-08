package tachyon

type Vars map[string]AnyValue

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
		o[k] = Any(inferString(v))
	}

	return o
}
