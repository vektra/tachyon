package tachyon

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSimplePlaybook(t *testing.T) {
	env := NewEnv(NewNestedScope(nil), DefaultConfig)
	p, err := NewPlaybook(env, "test/playbook1.yml")

	if err != nil {
		panic(err)
	}

	if len(p.Plays) != 2 {
		t.Fatalf("Didn't load 2 playbooks, loaded: %d", len(p.Plays))
	}

	x := p.Plays[1]

	if x.Hosts != "all" {
		t.Errorf("Hosts not all: was %s", x.Hosts)
	}

	vars := x.Vars

	a, ok := vars.Get("answer")

	if !ok {
		t.Fatalf("No var 'answer'")
	}

	if a.Read() != "Wuh, I think so" {
		t.Errorf("Unable to decode string var: %#v", a)
	}

	a, ok = vars.Get("port")

	if !ok {
		t.Fatalf("No var 'port'")
	}

	if a.Read() != 5150 {
		t.Errorf("Unable to decode numeric var: %#v", a)
	}

	if len(x.VarsFiles) != 2 {
		t.Fatalf("Unable to decode varsfiles, got %d", len(x.VarsFiles))
	}

	f := x.VarsFiles[0]

	if f != "common_vars.yml" {
		t.Errorf("Unable to decode literal vars_files")
	}

	f2 := x.VarsFiles[1].([]interface{})

	if f2[1].(string) != "default_os.yml" {
		t.Errorf("Unable to decode list vars_files")
	}

	tasks := x.Tasks

	if len(tasks) < 5 {
		t.Errorf("Failed to decode the proper number of tasks: %d", len(tasks))
	}

	if tasks[3].Args() != "echo {{port}}" {
		t.Errorf("Failed to decode templating in action: %#v", tasks[3].Args())
	}
}

func totalRuntime(results []RunResult) time.Duration {
	cur := time.Duration(0)

	for _, res := range results {
		cur += res.Runtime
	}

	return cur
}

func TestPlaybookFuturesRunInParallel(t *testing.T) {
	run, _, err := RunCapture("test/future.yml")
	if err != nil {
		t.Fatalf("Unable to load test/future.yml")
	}

	total := run.Runtime.Seconds()

	if total > 5.1 || total < 4.9 {
		t.Errorf("Futures did not run in parallel: %f", total)
	}
}

func TestPlaybookFuturesCanBeWaitedOn(t *testing.T) {
	run, _, err := RunCapture("test/future.yml")
	if err != nil {
		t.Fatalf("Unable to load test/future.yml")
	}

	total := run.Runtime.Seconds()

	if total > 5.1 || total < 4.9 {
		t.Errorf("Futures did not run in parallel: %f", total)
	}
}

func TestPlaybookTaskIncludes(t *testing.T) {
	res, _, err := RunCapture("test/inc_parent.yml")
	if err != nil {
		t.Fatalf("Unable to run test/inc_parent.yml")
	}

	// fmt.Printf("%#v\n", res.Results[0].Task.Play.File)

	if filepath.Base(res.Results[0].Task.File()) != "inc_child.yml" {
		t.Fatalf("Did not include tasks from child")
	}
}
