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
	Path    string
	baseDir string
	Plays   []*Play
	Env     *Environment
	Vars    *NestedScope
}

func NewPlaybook(env *Environment, p string) (*Playbook, error) {
	baseDir, err := filepath.Abs(filepath.Dir(p))
	if err != nil {
		return nil, err
	}

	pb := &Playbook{
		Path:    p,
		baseDir: baseDir,
		Env:     env,
		Vars:    NewNestedScope(env.Vars),
	}

	pb.Vars.Set("playbook_dir", baseDir)

	plays, err := pb.LoadPlays(p, pb.Vars)
	if err != nil {
		return nil, err
	}

	pb.Plays = plays

	return pb, nil
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

func (pb *Playbook) LoadPlays(fpath string, s Scope) ([]*Play, error) {
	var seq []map[string]interface{}

	var plays []*Play

	err := yamlFile(fpath, &seq)

	if err != nil {
		return nil, err
	}

	for _, item := range seq {
		if x, ok := item["include"]; ok {
			spath, ok := x.(string)

			if !ok {
				return nil, eInvalidPlaybook
			}

			// Make a new scope and put the vars into it. The subplays
			// will use this scope as their parent.
			ns := NewNestedScope(s)

			if vars, ok := item["vars"]; ok {
				ns.addVars(vars)
			}

			parts, err := shlex.Split(spath)
			if err == nil {
				spath = parts[0]
				for _, tok := range parts[1:] {
					if k, v, ok := split2(tok, "="); ok {
						ns.Set(k, inferString(v))
					}
				}
			}

			sub, err := pb.LoadPlays(path.Join(pb.baseDir, spath), ns.Flatten())

			if err != nil {
				return nil, err
			}

			if !ns.Empty() {
				for _, play := range sub {
					play.Vars = SpliceOverrides(play.Vars, ns)
				}
			}

			plays = append(plays, sub...)
		} else {
			play, err := parsePlay(s, pb.baseDir, item)

			if err != nil {
				return nil, err
			}

			plays = append(plays, play)
		}
	}

	return plays, nil
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

	if x, ok := m["vars_files"]; ok {
		if vf, ok := x.([]interface{}); ok {
			play.VarsFiles = vf
		} else {
			return nil, formatError("vars_files not the right format")
		}
	}

	play.baseDir = dir

	for _, file := range play.VarsFiles {
		switch file := file.(type) {
		case string:
			ImportVarsFile(play.Vars, play.path(file))
			break
		case []interface{}:
			for _, ent := range file {
				exp, err := ExpandVars(play.Vars, ent.(string))

				if err != nil {
					continue
				}

				epath := play.path(exp)

				if _, err := os.Stat(epath); err == nil {
					err = ImportVarsFile(play.Vars, epath)

					if err != nil {
						return nil, err
					}

					break
				}
			}
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

	play.Tasks = processTasks(tasks)
	play.Handlers = processTasks(handlers)

	return &play, nil
}

func (play *Play) path(file string) string {
	return path.Join(play.baseDir, file)
}

func boolify(str string) bool {
	switch str {
	case "", "false", "no":
		return false
	default:
		return true
	}
}
