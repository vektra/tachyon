package tachyon

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"time"
)

type Reporter interface {
	StartTasks(r *Runner)
	FinishTasks(r *Runner)
	StartHandlers(r *Runner)
	FinishHandlers(r *Runner)

	StartTask(task *Task, cmd Command, args string)
	FinishTask(task *Task, res *Result)

	FinishAsyncTask(act *AsyncAction)
}

type CLIReporter struct {
	out   io.Writer
	Start time.Time
}

var sCLIReporter *CLIReporter = &CLIReporter{out: os.Stdout}

func (c *CLIReporter) StartTasks(r *Runner) {
	c.Start = r.Start
	fmt.Fprintf(c.out, "== tasks @ %v\n", r.Start)
}

func (c *CLIReporter) FinishTasks(r *Runner) {
	fmt.Fprintf(c.out, "== Waiting on all tasks to finish...\n")
}

func (c *CLIReporter) StartHandlers(r *Runner) {
	fmt.Fprintf(c.out, "== Running any handlers\n")
}

func (c *CLIReporter) FinishHandlers(r *Runner) {}

func (c *CLIReporter) StartTask(task *Task, cmd Command, args string) {
	dur := time.Since(c.Start)

	if task.Async() {
		fmt.Fprintf(c.out, "%7.3f - %s &\n", dur.Seconds(), task.Name())
	} else {
		fmt.Fprintf(c.out, "%7.3f - %s\n", dur.Seconds(), task.Name())
	}

	if reflect.TypeOf(cmd).Elem().NumField() == 0 {
		fmt.Fprintf(c.out, "%7.3f  - %s: %s\n", dur.Seconds(), task.Command(), args)
	} else {
		fmt.Fprintf(c.out, "%7.3f  - %#v\n", dur.Seconds(), cmd)
		fmt.Fprintf(c.out, "%7.3f  - %s: %s\n", dur.Seconds(), task.Command(), args)
	}
}

func (c *CLIReporter) FinishTask(task *Task, res *Result) {
	if res == nil {
		return
	}

	dur := time.Since(c.Start)

	fmt.Fprintf(c.out, "%7.3f  - result:\n", dur.Seconds())

	indent := fmt.Sprintf("%7.3f      ", dur.Seconds())

	if sy, err := indentedYAML(res.Data, indent); err == nil {
		fmt.Fprintf(c.out, "%s", sy)
	}
}

func (c *CLIReporter) FinishAsyncTask(act *AsyncAction) {
	dur := time.Since(c.Start)

	if act.Error == nil {
		fmt.Fprintf(c.out, "%7.3f - %s (async success)\n", dur.Seconds(), act.Task.Name())
	} else {
		fmt.Fprintf(c.out, "%7.3f - %s (async error:%s)\n", dur.Seconds(), act.Task.Name(), act.Error)
	}
}
