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
	Roles      []string

	baseDir string
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

	defer env.SetPaths(env.SetPaths(SimplePath{baseDir}))

	plays, err := pb.LoadPlays(p, pb.Vars)
	if err != nil {
		return nil, err
	}

	pb.Plays = plays

	return pb, nil
}

type playData struct {
	Include    string
	Vars       map[string]interface{}
	Hosts      string
	Vars_files []interface{}
	Tasks      []TaskData
	Handlers   []TaskData
	Roles      []interface{}
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
			play, err := parsePlay(pb.Env, s, fpath, pb.baseDir, &item)

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

func (p *Play) importTasks(env *Environment, tasks *Tasks, file string, s Scope, tds []TaskData) error {
	for _, x := range tds {
		if _, ok := x["include"]; ok {
			err := p.decodeTasksFile(env, tasks, s, x)
			if err != nil {
				return err
			}
		} else {
			task := &Task{data: x, Play: p, File: file}
			task.Init(env)
			*tasks = append(*tasks, task)
		}
	}

	return nil
}

func (p *Play) decodeTasksFile(env *Environment, tasks *Tasks, s Scope, td TaskData) error {
	path := td["include"].(string)

	parts := strings.SplitN(path, " ", 2)

	path, err := ExpandVars(s, parts[0])
	if err != nil {
		return err
	}

	args := ""

	if len(parts) == 2 {
		args = parts[1]
	}

	filePath := env.Paths.Task(path)

	return p.importTasksFile(env, tasks, filePath, args, s, td)
}

func (p *Play) importTasksFile(env *Environment, tasks *Tasks, filePath string, args string, s Scope, td TaskData) error {

	var tds []TaskData

	err := yamlFile(filePath, &tds)
	if err != nil {
		return err
	}

	iv := make(Vars)

	if args != "" {
		sm, err := ParseSimpleMap(s, args)
		if err != nil {
			return err
		}

		for k, v := range sm {
			iv[k] = v
		}
	}

	// Inject yaml structured vars
	if xvars, ok := td["vars"]; ok {
		if cast, ok := xvars.(map[interface{}]interface{}); ok {
			for gk, gv := range cast {
				iv[gk.(string)] = Any(gv)
			}
		}
	}

	// Inject all additional keys
	for k, v := range td {
		switch k {
		case "include", "vars":
			continue
		default:
			iv[k] = Any(v)
		}
	}

	for _, x := range tds {
		if _, ok := x["include"]; ok {
			err := p.decodeTasksFile(env, tasks, s, x)
			if err != nil {
				return err
			}
		} else {
			task := &Task{data: x, Play: p, File: filePath}
			task.Init(env)
			task.IncludeVars = iv
			*tasks = append(*tasks, task)
		}
	}

	return nil
}

func (p *Play) importMeta(env *Environment, path string, s Scope) error {
	var m map[string]interface{}

	err := yamlFile(path, &m)
	if err != nil {
		return err
	}

	if deps, ok := m["dependencies"]; ok {
		if seq, ok := deps.([]interface{}); ok {
			for _, m := range seq {
				name, err := p.importRole(env, m, s)
				if err != nil {
					return err
				}

				p.Roles = append(p.Roles, name)
			}
		}
	}

	return nil
}

func (p *Play) importRole(env *Environment, o interface{}, s Scope) (string, error) {
	var role string

	ts := NewNestedScope(s)
	td := TaskData{}

	switch so := o.(type) {
	case string:
		role = so
	case map[interface{}]interface{}:
		for k, v := range so {
			sk := k.(string)

			if sk == "role" {
				role = v.(string)
			} else {
				ts.Set(sk, v)
				td[sk] = v
			}
		}
	default:
		return "", formatError("role not a map")
	}

	parts := strings.SplitN(role, " ", 2)

	if len(parts) == 2 {
		role = parts[0]

		sm, err := ParseSimpleMap(ts, parts[1])
		if err != nil {
			return "", err
		}

		for k, v := range sm {
			td[k] = v
		}
	}

	dir := env.Paths.Role(role)

	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("No role named %s available", role)
	}

	base := p.baseDir

	cur := env.Paths

	sep := SeparatePaths{Top: base, Root: cur.Role(role)}

	defer env.SetPaths(env.SetPaths(sep))

	metaPath := env.Paths.Meta("main.yml")

	dbg("meta: %s", metaPath)

	if fileExist(metaPath) {
		err := p.importMeta(env, metaPath, ts)
		if err != nil {
			return "", err
		}
	}

	taskPath := env.Paths.Task("main.yml")

	if fileExist(taskPath) {
		err := p.importTasksFile(env, &p.Tasks, taskPath, "", ts, td)
		if err != nil {
			return "", err
		}
	}

	handlers := env.Paths.Handler("main.yml")

	if fileExist(handlers) {
		err := p.importTasksFile(env, &p.Handlers, handlers, "", ts, td)
		if err != nil {
			return "", err
		}
	}

	vars := filepath.Join(base, "roles", role, "vars", "main.yml")

	if fileExist(vars) {
		err := ImportVarsFile(p.Vars, vars)
		if err != nil {
			return "", err
		}
	}

	return role, nil
}

func (play *Play) importVarsFiles(env *Environment) error {
	for _, file := range play.VarsFiles {
		switch file := file.(type) {
		case string:
			ImportVarsFile(play.Vars, env.Paths.Vars(file))
			break
		case []interface{}:
			for _, ent := range file {
				exp, err := ExpandVars(play.Vars, ent.(string))

				if err != nil {
					continue
				}

				epath := env.Paths.Vars(exp)

				if _, err := os.Stat(epath); err == nil {
					err = ImportVarsFile(play.Vars, epath)

					if err != nil {
						return err
					}

					break
				}
			}
		}
	}

	return nil
}

func parsePlay(env *Environment, s Scope, file, dir string, m *playData) (*Play, error) {
	var play Play

	if m.Hosts == "" {
		m.Hosts = "all"
	}

	play.Hosts = m.Hosts
	play.Vars = NewNestedScope(s)

	for sk, iv := range m.Vars {
		play.Vars.Set(sk, iv)
	}

	play.VarsFiles = m.Vars_files
	play.baseDir = dir

	err := play.importVarsFiles(env)
	if err != nil {
		return nil, err
	}

	if len(m.Tasks) > 0 {
		err := play.importTasks(env, &play.Tasks, file, s, m.Tasks)
		if err != nil {
			return nil, err
		}
	}

	if len(m.Handlers) > 0 {
		err := play.importTasks(env, &play.Handlers, file, s, m.Tasks)
		if err != nil {
			return nil, err
		}
	}

	for _, role := range m.Roles {
		name, err := play.importRole(env, role, s)
		if err != nil {
			return nil, err
		}

		play.Roles = append(play.Roles, name)
	}

	return &play, nil
}
