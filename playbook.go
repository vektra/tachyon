package tachyon

import (
	"fmt"
	"github.com/vektra/tachyon/lisp"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"
)

type Vars map[string]interface{}

type VarsFiles []interface{}

type Notifications []string

type TaskData map[string]interface{}

type PlayEnv struct {
	Vars      Vars
	lispScope *lisp.Scope
	to_notify map[string]struct{}
	async     chan *AsyncAction
	wait      sync.WaitGroup
}

func (pe *PlayEnv) Init() {
	pe.to_notify = make(map[string]struct{})
	pe.lispScope.AddEnv()
	pe.async = make(chan *AsyncAction)

	go pe.handleAsync()
}

func (pe *PlayEnv) Set(key string, val interface{}) {
	pe.Vars[key] = val

	switch lv := val.(type) {
	case int64:
		pe.lispScope.Set(key, lisp.NumberValue(lv))
	default:
		pe.lispScope.Set(key, lisp.StringValue(fmt.Sprintf("%s", lv)))
	}
}

func (pe *PlayEnv) Get(key string) (interface{}, bool) {
	v, ok := pe.Vars[key]

	return v, ok
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

type strmap map[string]interface{}

type Task struct {
	data TaskData
	cmd  string
	args string
	Vars strmap
}

var cOptions = []string{"name", "action", "notify", "async", "poll",
	"when"}

func (t *Task) Init() error {
	t.Vars = make(strmap)

	for k, v := range t.data {
		found := false

		for _, i := range cOptions {
			if k == i {
				found = true
				break
			}
		}

		if !found {
			t.cmd = k
			if m, ok := v.(map[interface{}]interface{}); ok {
				for ik, iv := range m {
					t.Vars[fmt.Sprintf("%v", ik)] = iv
				}
			}

			t.args = fmt.Sprintf("%v", v)
		}
	}

	if t.cmd == "" {
		act, ok := t.data["action"]
		if !ok {
			return fmt.Errorf("No action specified")
		}

		parts := strings.SplitN(fmt.Sprintf("%v", act), " ", 2)

		t.cmd = parts[0]
		t.args = parts[1]
	}

	return nil
}

func (t *Task) Command() string {
	return t.cmd
}

func (t *Task) Args() string {
	return t.args
}

func (t *Task) Name() string {
	return t.data["name"].(string)
}

func (t *Task) When() string {
	if v, ok := t.data["when"]; ok {
		return v.(string)
	}

	return ""
}

func (t *Task) Notify() []string {
	var v interface{}
	var ok bool

	if v, ok = t.data["notify"]; !ok {
		return nil
	}

	var list []interface{}

	if list, ok = v.([]interface{}); !ok {
		return nil
	}

	out := make([]string, len(list))

	for i, x := range list {
		out[i] = x.(string)
	}

	return out
}

func (t *Task) Async() bool {
	_, ok := t.data["async"]

	return ok
}

type Tasks []*Task

type Play struct {
	Hosts      string
	Connection string

	Vars      Vars
	VarsFiles VarsFiles `yaml:"vars_files"`

	TaskDatas []TaskData `yaml:"tasks"`
	Tasks     Tasks      `yaml:"-"`

	HandlerDatas []TaskData `yaml:"handlers"`
	Handlers     Tasks      `yaml:"-"`
}

type Playbook []*Play

func LoadPlaybook(path string) (Playbook, error) {
	var p Playbook

	data, err := ioutil.ReadFile(path)

	if err != nil {
		return nil, err
	}

	err = goyaml.Unmarshal(data, &p)

	for _, play := range p {
		tasks := make(Tasks, len(play.TaskDatas))

		for idx, data := range play.TaskDatas {
			task := &Task{data: data}
			task.Init()

			tasks[idx] = task
		}

		play.Tasks = tasks

		tasks = make(Tasks, len(play.HandlerDatas))

		for idx, data := range play.HandlerDatas {
			task := &Task{data: data}
			task.Init()

			tasks[idx] = task
		}

		play.Handlers = tasks
	}

	return p, err
}

func (p Playbook) Run(env *Environment) error {
	for _, play := range p {
		err := play.Run(env)

		if err != nil {
			return err
		}
	}

	return nil
}

func (play *Play) loadVarsFile(file string, pe *PlayEnv) error {
	var fv Vars

	data, err := ioutil.ReadFile(path.Join("test", file))

	if err != nil {
		return err
	}

	err = goyaml.Unmarshal(data, &fv)

	for k, v := range fv {
		pe.Set(k, v)
	}

	return nil
}

func (play *Play) Run(env *Environment) error {
	fmt.Printf("== tasks\n")

	pe := &PlayEnv{Vars: make(Vars), lispScope: lisp.NewScope()}
	pe.Init()

	for k, v := range play.Vars {
		pe.Set(k, v)
	}

	for _, file := range play.VarsFiles {
		switch file := file.(type) {
		case string:
			play.loadVarsFile(file, pe)
			break
		case []interface{}:
			for _, ent := range file {
				exp, err := env.ExpandVars(ent.(string), pe)

				if err != nil {
					continue
				}

				epath := path.Join("test", exp)

				if _, err := os.Stat(epath); err == nil {
					err = play.loadVarsFile(exp, pe)

					if err != nil {
						return err
					}

					break
				}
			}
		}
	}

	for _, task := range play.Tasks {
		err := task.Run(env, pe)

		if err != nil {
			return err
		}
	}

	fmt.Printf("== Waiting on all tasks to finish...\n")
	pe.wait.Wait()

	fmt.Printf("== Running any handlers\n")

	for _, task := range play.Handlers {
		if pe.ShouldRunHandler(task.Name()) {
			err := task.Run(env, pe)

			if err != nil {
				return err
			}
		}
	}

	return nil
}

func boolify(str string) bool {
	switch str {
	case "", "false", "no":
		return false
	default:
		return true
	}
}

func (task *Task) Run(env *Environment, pe *PlayEnv) error {
	if when := task.When(); when != "" {
		when, err := env.ExpandVars(when, pe)

		if err != nil {
			return err
		}

		if !boolify(when) {
			return nil
		}
	}

	str, err := env.ExpandVars(task.Args(), pe)

	if err != nil {
		return err
	}

	cmd, err := env.MakeCommand(task, pe, str)

	if err != nil {
		return err
	}

	if task.Async() {
		fmt.Printf("- %s &\n", task.Name())
	} else {
		fmt.Printf("- %s\n", task.Name())
	}

	if reflect.TypeOf(cmd).Elem().NumField() == 0 {
		fmt.Printf("  - %s: %s\n", task.Command(), str)
	} else {
		fmt.Printf("  - %#v\n  - %s: %s\n", cmd, task.Command(), str)
	}

	if task.Async() {
		asyncAction := &AsyncAction{Task: task}
		asyncAction.Init(pe)

		go func() {
			// fmt.Printf("Run %s => %s\n", parts[0], str)
			asyncAction.Finish(cmd.Run(env, pe, str))
		}()
	} else {
		// fmt.Printf("Run %s => %s\n", parts[0], str)
		err = cmd.Run(env, pe, str)

		if err == nil {
			for _, x := range task.Notify() {
				pe.AddNotify(x)
			}
		}
	}

	return err
}
