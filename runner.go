package tachyon

import (
	"sync"
	"time"
)

type RunResult struct {
	Task    *Task
	Result  *Result
	Runtime time.Duration
}

type Runner struct {
	env       *Environment
	plays     []*Play
	wait      sync.WaitGroup
	to_notify map[string]struct{}
	async     chan *AsyncAction
	report    Reporter

	Results []RunResult
	Runtime time.Duration
}

func NewRunner(env *Environment, plays []*Play) *Runner {
	r := &Runner{
		env:       env,
		plays:     plays,
		to_notify: make(map[string]struct{}),
		async:     make(chan *AsyncAction),
		report:    env.report,
	}

	go r.handleAsync()

	return r
}

func (r *Runner) SetReport(rep Reporter) {
	r.report = rep
}

func (r *Runner) AddNotify(n string) {
	r.to_notify[n] = struct{}{}
}

func (r *Runner) ShouldRunHandler(name string) bool {
	_, ok := r.to_notify[name]

	return ok
}

func (r *Runner) AsyncChannel() chan *AsyncAction {
	return r.async
}

func (r *Runner) Run(env *Environment) error {
	start := time.Now()
	defer func() {
		r.Runtime = time.Since(start)
	}()

	r.report.StartTasks(r)

	for _, play := range r.plays {
		fs := NewFutureScope(play.Vars)

		for _, task := range play.Tasks {
			err := r.runTask(env, task, fs)
			if err != nil {
				return err
			}
		}

		r.Results = append(r.Results, fs.Results()...)
	}

	r.report.FinishTasks(r)

	r.wait.Wait()

	r.report.StartHandlers(r)

	for _, play := range r.plays {
		fs := NewFutureScope(play.Vars)

		for _, task := range play.Handlers {
			if r.ShouldRunHandler(task.Name()) {
				err := r.runTask(env, task, fs)

				if err != nil {
					return err
				}
			}
		}

		fs.Wait()
	}

	r.report.FinishHandlers(r)

	return nil
}

func RunAdhocTask(cmd, args string) (*Result, error) {
	env := NewEnv(NewNestedScope(nil), &Config{})
	defer env.Cleanup()

	task := AdhocTask(cmd, args)

	str, err := ExpandVars(env.Vars, task.Args())
	if err != nil {
		return nil, err
	}

	obj, err := MakeCommand(env.Vars, task, str)
	if err != nil {
		return nil, err
	}

	ce := &CommandEnv{env, env.Paths}

	return obj.Run(ce, str)
}

type PriorityScope struct {
	task Vars
	rest Scope
}

func (p *PriorityScope) Get(key string) (Value, bool) {
	if p.task != nil {
		if v, ok := p.task[key]; ok {
			return Any(v), true
		}
	}

	return p.rest.Get(key)
}

func (p *PriorityScope) Set(key string, val interface{}) {
	p.rest.Set(key, val)
}

func (r *Runner) runTask(env *Environment, task *Task, fs *FutureScope) error {
	ps := &PriorityScope{task.IncludeVars, fs}

	start := time.Now()

	if when := task.When(); when != "" {
		when, err := ExpandVars(ps, when)

		if err != nil {
			return err
		}

		if !boolify(when) {
			return nil
		}
	}

	if items := task.Items(); items != nil {
		var results []*Result

		anyChanged := false

		for _, item := range items {
			ns := NewNestedScope(ps)
			ns.Set("item", item)

			str, err := ExpandVars(ns, task.Args())

			if err != nil {
				return err
			}

			cmd, err := MakeCommand(ns, task, str)

			if err != nil {
				return err
			}

			r.report.StartTask(task, cmd, str)

			ce := &CommandEnv{env, task.Paths}

			res, err := cmd.Run(ce, str)

			if err != nil {
				res = NewResult(false)
				res.Data.Set("failed", true)
				res.Data.Set("error", err.Error())
			}

			if res.Changed {
				anyChanged = true
			}

			results = append(results, res)
		}

		res := NewResult(anyChanged)
		res.Data.Set("items", len(items))
		res.Data.Set("results", results)

		if name := task.Register(); name != "" {
			fs.Set(name, res)
		}

		runtime := time.Since(start)

		r.Results = append(r.Results, RunResult{task, res, runtime})

		r.report.FinishTask(task, res)

		for _, x := range task.Notify() {
			r.AddNotify(x)
		}

		return nil
	}

	str, err := ExpandVars(ps, task.Args())

	if err != nil {
		return err
	}

	cmd, err := MakeCommand(ps, task, str)

	if err != nil {
		return err
	}

	r.report.StartTask(task, cmd, str)

	ce := &CommandEnv{env, task.Paths}

	if name := task.Future(); name != "" {
		future := NewFuture(start, task, func() (*Result, error) {
			return cmd.Run(ce, str)
		})

		fs.AddFuture(name, future)

		return nil
	}

	if task.Async() {
		asyncAction := &AsyncAction{Task: task}
		asyncAction.Init(r)

		go func() {
			asyncAction.Finish(cmd.Run(ce, str))
		}()
	} else {
		res, err := cmd.Run(ce, str)

		if name := task.Register(); name != "" {
			fs.Set(name, res)
		}

		runtime := time.Since(start)

		if err != nil {
			res = NewResult(false)
			res.Data.Set("failed", true)
			res.Data.Set("error", err.Error())
		}

		r.Results = append(r.Results, RunResult{task, res, runtime})

		r.report.FinishTask(task, res)

		if err == nil {
			for _, x := range task.Notify() {
				r.AddNotify(x)
			}
		}
	}

	return err
}
