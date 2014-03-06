package tachyon

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type ResultData map[string]Value

func (rd ResultData) Set(key string, v interface{}) {
	rd[key] = Any{v}
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

func (r *Result) Get(key string) (Value, bool) {
	v, ok := r.Data[key]

	return v, ok
}

func (r *Result) Add(key string, v interface{}) {
	r.Data[key] = Any{v}
}

func WrapResult(changed bool, data ResultData) *Result {
	return &Result{changed, data}
}

func NewResult(changed bool) *Result {
	return &Result{changed, make(ResultData)}
}

type CommandEnv struct {
	Env   *Environment
	Paths Paths
}

type Command interface {
	Run(env *CommandEnv, args string) (*Result, error)
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

func MakeCommand(s Scope, task *Task, args string) (Command, error) {
	name := task.Command()

	t, ok := AvailableCommands[name]

	if !ok {
		return nil, fmt.Errorf("Unknown command: %s", name)
	}

	obj := reflect.New(t)

	sm, err := ParseSimpleMap(s, args)

	if err == nil {
		for ik, iv := range task.Vars {
			exp, err := ExpandVars(s, fmt.Sprintf("%v", iv))
			if err != nil {
				return nil, err
			}

			sm[ik] = exp
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
					return nil, fmt.Errorf("Unsupported tag flag: %s", parts[1])
				}
			}

			if val, ok := sm[name]; ok {
				ef := e.Field(i)

				if _, ok := ef.Interface().(bool); ok {
					e.Field(i).Set(reflect.ValueOf(boolify(val)))
				} else {
					e.Field(i).Set(reflect.ValueOf(fmt.Sprintf("%v", val)))
				}
			} else if required {
				return nil, fmt.Errorf("Missing value for %s", f.Name)
			}
		}
	}

	return obj.Interface().(Command), nil
}
