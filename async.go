package tachyon

import (
	"fmt"
)

type AsyncAction struct {
	Task   *Task
	Error  error
	Result *Result
	status chan *AsyncAction
}

func (a *AsyncAction) Init(r *Runner) {
	r.wait.Add(1)
	a.status = r.AsyncChannel()
}

func (a *AsyncAction) Finish(res *Result, err error) {
	a.Error = err
	a.Result = res
	a.status <- a
}

func (r *Runner) handleAsync() {
	for {
		act := <-r.async

		if act.Error == nil {
			fmt.Printf("- %s (async success)\n", act.Task.Name())

			for _, x := range act.Task.Notify() {
				r.AddNotify(x)
			}
		} else {
			fmt.Printf("- %s (async error:%s)\n", act.Task.Name(), act.Error)
		}

		r.wait.Done()
	}
}
