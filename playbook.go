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

func (p *Play) processTasks(datas []TaskData) Tasks {
	tasks := make(Tasks, len(datas))

	for idx, data := range datas {
		task := &Task{data: data, Play: p}
		task.Init()

		tasks[idx] = task
	}

	return tasks
}

type playData struct {
	Include    string
	Vars       strmap
	Hosts      string
	Vars_files []interface{}
	Tasks      []TaskData
	Handlers   []TaskData
}

var eInvalidPlaybook = errors.New("Invalid playbook yaml")

func (pb *Playbook) LoadPlays(fpath string, s Scope) ([]*Play, error) {
	var seq []playData

	var plays []*Play

	err := yamlFile(fpath, &seq)

	if err != nil {
		return nil, err
	}

	for _, item := range seq {
		if item.Include != "" {
			spath := item.Include

			// Make a new scope and put the vars into it. The subplays
			// will use this scope as their parent.
			ns := NewNestedScope(s)

			if item.Vars != nil {
				ns.addVars(item.Vars)
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
			play, err := parsePlay(s, fpath, pb.baseDir, &item)

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

func (p *Play) castTasks(s Scope, file string, t []TaskData) ([]TaskData, error) {
	var tds []TaskData

	for _, x := range t {
		if _, ok := x["include"]; ok {
			tasks, err := p.loadTasksFile(s, x)
			if err != nil {
				return nil, err
			}

			tds = append(tds, tasks...)
		} else {
			x[":file"] = file
			tds = append(tds, x)
		}
	}

	return tds, nil
}

func (p *Play) loadTasksFile(s Scope, td TaskData) ([]TaskData, error) {
	path, ok := td["include"].(string)
	if !ok {
		return nil, formatError("include was not a string")
	}

	path, err := ExpandVars(s, path)
	if err != nil {
		return nil, err
	}

	var tds []TaskData

	filePath := p.path(path)

	err = yamlFile(filePath, &tds)

	for _, td := range tds {
		td[":file"] = filePath
	}

	return tds, err
}

func parsePlay(s Scope, file, dir string, m *playData) (*Play, error) {
	var play Play

	if m.Hosts == "" {
		return nil, formatError("hosts missing")
	}

	play.Hosts = m.Hosts
	play.Vars = NewNestedScope(s)

	for sk, iv := range m.Vars {
		play.Vars.Set(sk, iv)
	}

	play.VarsFiles = m.Vars_files
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
	if len(m.Tasks) > 0 {
		tds, err := play.castTasks(s, file, m.Tasks)

		if err != nil {
			return nil, err
		}

		tasks = tds
	}

	var handlers []TaskData

	if len(m.Handlers) > 0 {
		tds, err := play.castTasks(s, file, m.Handlers)

		if err != nil {
			return nil, err
		}

		handlers = tds
	}

	play.Tasks = play.processTasks(tasks)
	play.Handlers = play.processTasks(handlers)

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
