package tachyon

import (
	"sync"
)

type Runner struct {
	env       *Environment
	plays     []*Play
	wait      sync.WaitGroup
	to_notify map[string]struct{}
	async     chan *AsyncAction
	report    Reporter

	Results []*Result
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
	r.report.StartTasks(r)

	for _, play := range r.plays {
		fs := NewFutureScope(play.Vars)

		for _, task := range play.Tasks {
			err := r.runTask(env, task, fs)
			if err != nil {
				return err
			}
		}

		fs.Wait()
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

	return obj.Run(env, str)
}

func (r *Runner) runTask(env *Environment, task *Task, fs *FutureScope) error {
	if when := task.When(); when != "" {
		when, err := ExpandVars(fs, when)

		if err != nil {
			return err
		}

		if !boolify(when) {
			return nil
		}
	}

	str, err := ExpandVars(fs, task.Args())

	if err != nil {
		return err
	}

	cmd, err := MakeCommand(fs, task, str)

	if err != nil {
		return err
	}

	r.report.StartTask(task, cmd, str)

	if name := task.Future(); name != "" {
		future := NewFuture(func() (*Result, error) {
			return cmd.Run(env, str)
		})

		fs.AddFuture(name, future)

		return nil
	}

	if task.Async() {
		asyncAction := &AsyncAction{Task: task}
		asyncAction.Init(r)

		go func() {
			asyncAction.Finish(cmd.Run(env, str))
		}()
	} else {
		res, err := cmd.Run(env, str)

		if name := task.Register(); name != "" {
			fs.Set(name, res)
		}

		r.Results = append(r.Results, res)

		r.report.FinishTask(task, res)

		if err == nil {
			for _, x := range task.Notify() {
				r.AddNotify(x)
			}
		}
	}

	return err
}
