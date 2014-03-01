package tachyon

import (
	"fmt"
)

type Value interface {
	Read() interface{}
}

type Any struct {
	v interface{}
}

func (a Any) Read() interface{} {
	return a.v
}

type Map interface {
	Get(key string) (Value, bool)
}

type Scope interface {
	Get(key string) (Value, bool)
	Set(key string, val interface{})
}

func SV(v interface{}, ok bool) interface{} {
	if !ok {
		return nil
	}

	return v
}

type NestedScope struct {
	Scope Scope
	Vars  Vars
}

func NewNestedScope(parent Scope) *NestedScope {
	return &NestedScope{parent, make(Vars)}
}

func SpliceOverrides(cur Scope, override *NestedScope) *NestedScope {
	ns := NewNestedScope(cur)

	for k, v := range override.Vars {
		ns.Set(k, v)
	}

	return ns
}

func (n *NestedScope) Get(key string) (v Value, ok bool) {
	v, ok = n.Vars[key]
	if !ok && n.Scope != nil {
		v, ok = n.Scope.Get(key)
	}

	return
}

func (n *NestedScope) Set(key string, v interface{}) {
	n.Vars[key] = Any{v}
}

func (n *NestedScope) Empty() bool {
	return len(n.Vars) == 0
}

func (n *NestedScope) Flatten() Scope {
	if len(n.Vars) == 0 && n.Scope != nil {
		return n.Scope
	}

	return n
}

func (n *NestedScope) addMapVars(mv map[interface{}]interface{}) {
	for k, v := range mv {
		if sk, ok := k.(string); ok {
			n.Set(sk, v)
		}
	}
}

func (n *NestedScope) addVars(vars interface{}) {
	switch mv := vars.(type) {
	case map[interface{}]interface{}:
		n.addMapVars(mv)
	case []interface{}:
		for _, i := range mv {
			n.addVars(i)
		}
	}
}

func ImportVarsFile(s Scope, path string) error {
	var fv Vars

	err := yamlFile(path, &fv)

	if err != nil {
		return err
	}

	for k, v := range fv {
		s.Set(k, v)
	}

	return nil
}

func DisplayScope(s Scope) {
	if ns, ok := s.(*NestedScope); ok {
		DisplayScope(ns.Scope)

		for k, v := range ns.Vars {
			fmt.Printf("%s: %v\n", k, v)
		}
	}
}
