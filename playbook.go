package tachyon

import (
	"errors"
	"fmt"
	"github.com/flynn/go-shlex"
	"os"
	"path"
	"path/filepath"
)

type VarsFiles []interface{}

type Notifications []string

type TaskData map[string]interface{}

type Play struct {
	Hosts      string
	Connection string
	Vars       Scope
	VarsFiles  VarsFiles
	Tasks      Tasks
	Handlers   Tasks
	baseDir    string
}

type Playbook struct {
	Plays []*Play
	Env   *Environment
}

func processTasks(datas []TaskData) Tasks {
	tasks := make(Tasks, len(datas))

	for idx, data := range datas {
		task := &Task{data: data}
		task.Init()

		tasks[idx] = task
	}

	return tasks
}

var eInvalidPlaybook = errors.New("Invalid playbook yaml")

func LoadPlaybook(fpath string, s Scope, env *Environment) (*Playbook, error) {
	baseDir, err := filepath.Abs(filepath.Dir(fpath))

	pbs := NewNestedScope(s)

	pbs.Set("playbook_dir", baseDir)

	if err != nil {
		return nil, err
	}

	var seq []map[string]interface{}

	err = yamlFile(fpath, &seq)

	if err != nil {
		return nil, err
	}

	p := &Playbook{Env: env}

	for _, item := range seq {
		if x, ok := item["include"]; ok {
			var sub *Playbook

			spath, ok := x.(string)

			if !ok {
				return nil, eInvalidPlaybook
			}

			parts, err := shlex.Split(spath)
			if err == nil {
				spath = parts[0]
			}

			// Make a new scope and put the vars into it. The subplays
			// will use this scope as their parent.
			ns := NewNestedScope(pbs)

			if vars, ok := item["vars"]; ok {
				ns.addVars(vars)
			}

			for _, tok := range parts[1:] {
				if k, v, ok := split2(tok, "="); ok {
					ns.Set(k, inferString(v))
				}
			}

			var us *NestedScope

			if ns.Empty() {
				us = pbs
			} else {
				us = ns
			}

			sub, err = LoadPlaybook(path.Join(baseDir, spath), us, env)

			if err != nil {
				return nil, err
			}

			if !ns.Empty() {
				for _, play := range sub.Plays {
					play.Vars = SpliceOverrides(play.Vars, ns)
				}
			}

			p.Plays = append(p.Plays, sub.Plays...)
		} else {
			play, err := parsePlay(pbs, baseDir, item)

			if err != nil {
				return nil, err
			}

			p.Plays = append(p.Plays, play)
		}
	}

	return p, nil
}

func formatError(where string) error {
	return fmt.Errorf("Invalid playbook yaml: %s", where)
}

func castTasks(x interface{}) ([]TaskData, error) {
	if xs, ok := x.([]interface{}); ok {
		var tds []TaskData

		for _, x := range xs {
			if am, ok := x.(map[interface{}]interface{}); ok {
				td := make(TaskData)

				for k, v := range am {
					if sk, ok := k.(string); ok {
						td[sk] = v
					} else {
						return nil, formatError("non-string key in task")
					}
				}

				tds = append(tds, td)
			} else {
				return nil, formatError("task was not a map")
			}
		}

		return tds, nil
	} else {
		return nil, formatError("tasks not the right format")
	}
}

func parsePlay(s Scope, dir string, m map[string]interface{}) (*Play, error) {
	var play Play

	if x, ok := m["hosts"]; ok {
		if str, ok := x.(string); ok {
			play.Hosts = str
		} else {
			return nil, formatError("hosts not a string")
		}
	} else {
		return nil, formatError("hosts missing")
	}

	play.Vars = NewNestedScope(s)

	if x, ok := m["vars"]; ok {
		if im, ok := x.(map[interface{}]interface{}); ok {
			for ik, iv := range im {
				if sk, ok := ik.(string); ok {
					play.Vars.Set(sk, iv)
				} else {
					return nil, formatError("vars key not a string")
				}
			}
		} else {
			return nil, formatError("vars not a map")
		}
	}

	var tasks []TaskData

	if x, ok := m["tasks"]; ok {
		tds, err := castTasks(x)

		if err != nil {
			return nil, err
		}

		tasks = tds
	}

	var handlers []TaskData

	if x, ok := m["handlers"]; ok {
		tds, err := castTasks(x)

		if err != nil {
			return nil, err
		}

		handlers = tds
	}

	if x, ok := m["vars_files"]; ok {
		if vf, ok := x.([]interface{}); ok {
			play.VarsFiles = vf
		} else {
			return nil, formatError("vars_files not the right format")
		}
	}

	play.baseDir = dir
	play.Tasks = processTasks(tasks)
	play.Handlers = processTasks(handlers)

	return &play, nil
}

func (p *Playbook) Run(env *Environment) error {
	for _, play := range p.Plays {
		err := play.Run(env)

		if err != nil {
			return err
		}
	}

	return nil
}

func (play *Play) path(file string) string {
	return path.Join(play.baseDir, file)
}

func (play *Play) Run(env *Environment) error {
	env.report.StartTasks(play)

	pe := &PlayEnv{}
	pe.Init(play, env)

	for _, file := range play.VarsFiles {
		switch file := file.(type) {
		case string:
			ImportVarsFile(pe.Vars, play.path(file))
			break
		case []interface{}:
			for _, ent := range file {
				exp, err := ExpandVars(pe.Vars, ent.(string))

				if err != nil {
					continue
				}

				epath := play.path(exp)

				if _, err := os.Stat(epath); err == nil {
					err = ImportVarsFile(pe.Vars, epath)

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
		when, err := ExpandVars(pe.Vars, when)

		if err != nil {
			return err
		}

		if !boolify(when) {
			return nil
		}
	}

	str, err := ExpandVars(pe.Vars, task.Args())

	if err != nil {
		return err
	}

	cmd, err := pe.MakeCommand(task, str)

	if err != nil {
		return err
	}

	pe.report.StartTask(task, cmd, str)

	if task.Async() {
		asyncAction := &AsyncAction{Task: task}
		asyncAction.Init(pe)

		go func() {
			asyncAction.Finish(cmd.Run(pe, str))
		}()
	} else {
		err = cmd.Run(pe, str)

		pe.report.FinishTask(task, false)

		if err == nil {
			for _, x := range task.Notify() {
				pe.AddNotify(x)
			}
		}
	}

	return err
}
