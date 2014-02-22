package tachyon

import (
	"sync"
)

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

type PlayEnv struct {
	env       *Environment
	Vars      Scope
	to_notify map[string]struct{}
	async     chan *AsyncAction
	wait      sync.WaitGroup
	report    Reporter
	config    *Config
}

func (pe *PlayEnv) Init(play *Play, env *Environment) {
	pe.env = env
	pe.to_notify = make(map[string]struct{})
	pe.async = make(chan *AsyncAction)
	pe.report = env.report
	pe.config = env.config
	pe.Vars = NewNestedScope(play.Vars)

	go pe.handleAsync()
}

func (pe *PlayEnv) AddNotify(n string) {
	pe.to_notify[n] = struct{}{}
}

func (pe *PlayEnv) ShouldRunHandler(name string) bool {
	_, ok := pe.to_notify[name]

	return ok
}

func (pe *PlayEnv) AsyncChannel() chan *AsyncAction {
	return pe.async
}

func (pe *PlayEnv) ImportVars(vars Vars) {
	for k, v := range vars {
		pe.Vars.Set(k, v)
	}
}

func ImportVarsFile(s Scope, path string) error {
	var fv Vars

	err := yamlFile(path, &fv)

	if err != nil {
		return err
	}

	for k, v := range fv {
		s.Set(k, v)
	}

	return nil
}
