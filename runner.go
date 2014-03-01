package tachyon

import (
	"sync"
)

type Runner struct {
	plays     []*Play
	wait      sync.WaitGroup
	to_notify map[string]struct{}
	async     chan *AsyncAction
}

func NewRunner(plays []*Play) *Runner {
	r := &Runner{
		plays:     plays,
		to_notify: make(map[string]struct{}),
		async:     make(chan *AsyncAction),
	}

	go r.handleAsync()

	return r
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
	env.report.StartTasks(r)

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

	env.report.FinishTasks(r)

	r.wait.Wait()

	env.report.StartHandlers(r)

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

	env.report.FinishHandlers(r)

	return nil
}

func RunAdhocTask(cmd, args string) (*Result, error) {
	env := &Environment{Vars: NewNestedScope(nil)}
	env.config = &Config{}

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

	env.report.StartTask(task, cmd, str)

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

		env.report.FinishTask(task, false)

		if err == nil {
			for _, x := range task.Notify() {
				r.AddNotify(x)
			}
		}
	}

	return err
}
