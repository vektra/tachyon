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

func (f *Future) Wait() {
	f.wg.Wait()
}

func (f *Future) Value() (*Result, error) {
	f.Wait()
	return f.result, f.err
}

func (f *Future) Read() interface{} {
	f.Wait()
	return f.result
}

type Futures map[string]*Future

type FutureScope struct {
	Scope
	futures Futures
}

func NewFutureScope(parent Scope) *FutureScope {
	return &FutureScope{
		Scope:   parent,
		futures: Futures{},
	}
}

func (fs *FutureScope) Get(key string) (Value, bool) {
	if v, ok := fs.futures[key]; ok {
		return v, ok
	}

	return fs.Scope.Get(key)
}

func (fs *FutureScope) AddFuture(key string, f *Future) {
	fs.futures[key] = f
}

func (fs *FutureScope) Wait() {
	for _, f := range fs.futures {
		f.Wait()
	}
}
