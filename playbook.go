package tachyon

import (
	"errors"
	"fmt"
	"github.com/flynn/go-shlex"
	"os"
	"path"
	"path/filepath"
	"strings"
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

func (p *Play) importTasks(tasks *Tasks, file string, s Scope, tds []TaskData) error {
	for _, x := range tds {
		if _, ok := x["include"]; ok {
			err := p.importTasksFile(tasks, s, x)
			if err != nil {
				return err
			}
		} else {
			task := &Task{data: x, Play: p, File: file}
			task.Init()
			*tasks = append(*tasks, task)
		}
	}

	return nil
}

func (p *Play) importTasksFile(tasks *Tasks, s Scope, td TaskData) error {
	path, ok := td["include"].(string)
	if !ok {
		return formatError("include was not a string")
	}

	parts := strings.SplitN(path, " ", 2)

	path, err := ExpandVars(s, parts[0])
	if err != nil {
		return err
	}

	filePath := p.path(path)

	var tds []TaskData

	err = yamlFile(filePath, &tds)
	if err != nil {
		return err
	}

	var iv strmap

	if len(parts) == 2 {
		sm, err := ParseSimpleMap(s, parts[1])
		if err != nil {
			return err
		}

		iv = make(strmap)

		for k, v := range sm {
			iv[k] = inferString(v)
		}
	}

	if xvars, ok := td["vars"]; ok {
		if cast, ok := xvars.(map[interface{}]interface{}); ok {
			for gk, gv := range cast {
				iv[gk.(string)] = gv
			}
		}
	}

	for _, x := range tds {
		if _, ok := x["include"]; ok {
			err := p.importTasksFile(tasks, s, x)
			if err != nil {
				return err
			}
		} else {
			task := &Task{data: x, Play: p, File: path}
			task.Init()
			task.IncludeVars = iv
			*tasks = append(*tasks, task)
		}
	}

	return nil
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

	if len(m.Tasks) > 0 {
		err := play.importTasks(&play.Tasks, file, s, m.Tasks)
		if err != nil {
			return nil, err
		}
	}

	if len(m.Handlers) > 0 {
		err := play.importTasks(&play.Handlers, file, s, m.Tasks)
		if err != nil {
			return nil, err
		}
	}

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
