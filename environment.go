package tachyon

type Environment struct {
	Vars   Scope
	report Reporter
	config *Config
}

func (e *Environment) Init(s Scope, cfg *Config) {
	e.report = sCLIReporter
	e.Vars = s
	e.config = cfg
}
