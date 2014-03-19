package tachyon

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type ResultData map[string]Value

func (rd ResultData) Set(key string, v interface{}) {
	rd[key] = Any(v)
}

func (rd ResultData) Get(key string) interface{} {
	if a, ok := rd[key]; !ok {
		return nil
	} else {
		return a.Read()
	}
}

type Result struct {
	Changed bool
	Data    ResultData
}

func (r *Result) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	m["changed"] = r.Changed

	for k, v := range r.Data {
		m[k] = v.Read()
	}

	return json.Marshal(m)
}

func (r *Result) Get(key string) (Value, bool) {
	v, ok := r.Data[key]

	return v, ok
}

func (r *Result) Add(key string, v interface{}) {
	r.Data[key] = Any(v)
}

func WrapResult(changed bool, data ResultData) *Result {
	return &Result{changed, data}
}

func NewResult(changed bool) *Result {
	return &Result{changed, make(ResultData)}
}

type CommandEnv struct {
	Env      *Environment
	Paths    Paths
	progress ProgressReporter
}

func NewCommandEnv(env *Environment, task *Task) *CommandEnv {
	return &CommandEnv{
		Env:      env,
		Paths:    task.Paths,
		progress: env.report,
	}
}

func (e *CommandEnv) Progress(str string) {
	if e.progress == nil {
		fmt.Printf("=== %s\n", str)
	} else {
		e.progress.Progress(str)
	}
}

type Command interface {
	Run(env *CommandEnv, args string) (*Result, error)
}

type ArgParser interface {
	ParseArgs(s Scope, args string) (Vars, error)
}

type Commands map[string]reflect.Type

var AvailableCommands Commands

var initAvailable sync.Once

func RegisterCommand(name string, cmd Command) {
	initAvailable.Do(func() {
		AvailableCommands = make(Commands)
	})

	ref := reflect.ValueOf(cmd)
	e := ref.Elem()

	AvailableCommands[name] = e.Type()
}

func MakeCommand(s Scope, task *Task, args string) (Command, Vars, error) {
	name := task.Command()

	t, ok := AvailableCommands[name]

	if !ok {
		return nil, nil, fmt.Errorf("Unknown command: %s", name)
	}

	obj := reflect.New(t)

	var sm Vars
	var err error

	if ap, ok := obj.Interface().(ArgParser); ok {
		sm, err = ap.ParseArgs(s, args)
	} else {
		sm, err = ParseSimpleMap(s, args)
	}

	if err != nil {
		return nil, nil, err
	}

	for ik, iv := range task.Vars {
		sm[ik] = iv
	}

	e := obj.Elem()

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		name := strings.ToLower(f.Name)
		required := false

		parts := strings.Split(f.Tag.Get("tachyon"), ",")

		switch len(parts) {
		case 0:
			// nothing
		case 1:
			name = parts[0]
		case 2:
			name = parts[0]
			switch parts[1] {
			case "required":
				required = true
			default:
				return nil, nil, fmt.Errorf("Unsupported tag flag: %s", parts[1])
			}
		}

		if val, ok := sm[name]; ok {
			ef := e.Field(i)

			if _, ok := ef.Interface().(bool); ok {
				e.Field(i).Set(reflect.ValueOf(val.Read()))
			} else {
				val := fmt.Sprintf("%v", val.Read())
				enum := f.Tag.Get("enum")
				if enum != "" {
					found := false

					for _, p := range strings.Split(enum, ",") {
						if p == val {
							found = true
							break
						}
					}

					if !found {
						return nil, nil, fmt.Errorf("Invalid value '%s' for variable '%s'. Possibles: %s", val, name, enum)
					}
				}

				e.Field(i).Set(reflect.ValueOf(val))
			}
		} else if required {
			return nil, nil, fmt.Errorf("Missing value for %s", f.Name)
		}
	}

	return obj.Interface().(Command), sm, nil
}
