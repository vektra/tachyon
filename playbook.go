package tachyon

import (
	"github.com/vektra/tachyon/lisp"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"path"
	"path/filepath"
)

type Vars map[string]interface{}

type VarsFiles []interface{}

type Notifications []string

type TaskData map[string]interface{}

type Play struct {
	Hosts      string
	Connection string

	Vars      Vars
	VarsFiles VarsFiles `yaml:"vars_files"`

	TaskDatas []TaskData `yaml:"tasks"`
	Tasks     Tasks      `yaml:"-"`

	HandlerDatas []TaskData `yaml:"handlers"`
	Handlers     Tasks      `yaml:"-"`

	baseDir string
}

type Playbook []*Play

func LoadPlaybook(path string) (Playbook, error) {
	var p Playbook

	data, err := ioutil.ReadFile(path)

	if err != nil {
		return nil, err
	}

	baseDir, err := filepath.Abs(filepath.Dir(path))

	if err != nil {
		return nil, err
	}

	err = goyaml.Unmarshal(data, &p)

	for _, play := range p {
		play.baseDir = baseDir

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

	data, err := ioutil.ReadFile(path.Join(play.baseDir, file))

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
	env.report.StartTasks(play)

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

				epath := path.Join(play.baseDir, exp)

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

	env.report.FinishTasks(play)

	pe.wait.Wait()

	env.report.StartHandlers(play)

	for _, task := range play.Handlers {
		if pe.ShouldRunHandler(task.Name()) {
			err := task.Run(env, pe)

			if err != nil {
				return err
			}
		}
	}

	env.report.FinishHandlers(play)

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

	env.report.StartTask(task, cmd, str)

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

		env.report.FinishTask(task, false)

		if err == nil {
			for _, x := range task.Notify() {
				pe.AddNotify(x)
			}
		}
	}

	return err
}
