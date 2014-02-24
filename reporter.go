package tachyon

import (
	"fmt"
	"reflect"
)

type Reporter interface {
	StartTasks(r *Runner)
	FinishTasks(r *Runner)
	StartHandlers(r *Runner)
	FinishHandlers(r *Runner)

	StartTask(task *Task, cmd Command, args string)
	FinishTask(task *Task, async bool)
}

type CLIReporter struct{}

var sCLIReporter *CLIReporter = &CLIReporter{}

func (c *CLIReporter) StartTasks(r *Runner) {
	fmt.Printf("== tasks\n")
}

func (c *CLIReporter) FinishTasks(r *Runner) {
	fmt.Printf("== Waiting on all tasks to finish...\n")
}

func (c *CLIReporter) StartHandlers(r *Runner) {
	fmt.Printf("== Running any handlers\n")
}

func (c *CLIReporter) FinishHandlers(r *Runner) {}

func (c *CLIReporter) StartTask(task *Task, cmd Command, args string) {
	if task.Async() {
		fmt.Printf("- %s &\n", task.Name())
	} else {
		fmt.Printf("- %s\n", task.Name())
	}

	if reflect.TypeOf(cmd).Elem().NumField() == 0 {
		fmt.Printf("  - %s: %s\n", task.Command(), args)
	} else {
		fmt.Printf("  - %#v\n  - %s: %s\n", cmd, task.Command(), args)
	}
}

func (c *CLIReporter) FinishTask(task *Task, async bool) {}
