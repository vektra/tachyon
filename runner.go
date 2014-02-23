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
		for _, task := range play.Tasks {
			err := r.runTask(env, task, play.Vars)
			if err != nil {
				return err
			}
		}
	}

	env.report.FinishTasks(r)

	r.wait.Wait()

	env.report.StartHandlers(r)

	for _, play := range r.plays {
		for _, task := range play.Handlers {
			if r.ShouldRunHandler(task.Name()) {
				err := r.runTask(env, task, play.Vars)

				if err != nil {
					return err
				}
			}
		}
	}

	env.report.FinishHandlers(r)

	return nil
}

func (r *Runner) runTask(env *Environment, task *Task, s Scope) error {
	if when := task.When(); when != "" {
		when, err := ExpandVars(s, when)

		if err != nil {
			return err
		}

		if !boolify(when) {
			return nil
		}
	}

	str, err := ExpandVars(s, task.Args())

	if err != nil {
		return err
	}

	cmd, err := MakeCommand(s, task, str)

	if err != nil {
		return err
	}

	env.report.StartTask(task, cmd, str)

	if task.Async() {
		asyncAction := &AsyncAction{Task: task}
		asyncAction.Init(r)

		go func() {
			asyncAction.Finish(cmd.Run(env, str))
		}()
	} else {
		err = cmd.Run(env, str)

		env.report.FinishTask(task, false)

		if err == nil {
			for _, x := range task.Notify() {
				r.AddNotify(x)
			}
		}
	}

	return err
}
