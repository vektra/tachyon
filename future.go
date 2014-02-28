package tachyon

import (
	"sync"
)

type Future struct {
	result *Result
	err    error
	wg     sync.WaitGroup
}

func NewFuture(f func() (*Result, error)) *Future {
	fut := &Future{}

	fut.wg.Add(1)

	go func() {
		r, e := f()
		fut.result = r
		fut.err = e
		fut.wg.Done()
	}()

	return fut
}

func (f *Future) Value() (*Result, error) {
	f.wg.Wait()
	return f.result, f.err
}

type FutureScope struct {
	Scope
	futures map[string]*Future
}

func NewFutureScope(parent Scope) *FutureScope {
	return &FutureScope{
		Scope:   parent,
		futures: make(map[string]*Future),
	}
}

func (fs *FutureScope) Get(key string) (interface{}, bool) {
	if v, ok := fs.futures[key]; ok {
		return v, ok
	}

	return fs.Scope.Get(key)
}

func (fs *FutureScope) AddFuture(key string, f *Future) {
	fs.futures[key] = f
}
