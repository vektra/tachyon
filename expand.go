package tachyon

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/vektra/tachyon/lisp"
	"unicode"
)

var cTemplateStart = []byte(`{{`)
var cTemplateEnd = []byte(`}}`)
var cExprStart = []byte(`$(`)
var cExprEnd = []byte(`)`)

var eUnclosedTemplate = errors.New("Unclosed template")
var eUnclosedExpr = errors.New("Unclosed lisp expression")

func expandTemplates(s Scope, args string) (string, error) {
	a := []byte(args)

	var buf bytes.Buffer

	for {
		idx := bytes.Index(a, cTemplateStart)

		if idx == -1 {
			buf.Write(a)
			break
		}

		buf.Write(a[:idx])

		in := a[idx+2:]

		fin := bytes.Index(in, cTemplateEnd)

		if fin == -1 {
			return "", eUnclosedTemplate
		}

		name := bytes.TrimSpace(in[:fin])

		if val, ok := s.Get(string(name)); ok {
			switch val := val.(type) {
			case int64, int:
				buf.WriteString(fmt.Sprintf("%d", val))
			default:
				buf.WriteString(fmt.Sprintf("%s", val))
			}

			a = in[fin+2:]
		} else {
			return "", fmt.Errorf("Undefined variable: %s", string(name))
		}
	}

	return buf.String(), nil
}

func findExprClose(buf []byte) int {
	opens := 0

	for idx, r := range buf {
		switch r {
		case ')':
			opens--

			if opens == 0 {
				return idx
			}

		case '(':
			opens++
		}
	}

	return -1
}

func varChar(r rune) bool {
	if unicode.IsLetter(r) {
		return true
	}
	if unicode.IsDigit(r) {
		return true
	}
	if r == '_' {
		return true
	}
	return false
}

type lispInferredScope struct {
	Scope Scope
}

func (s lispInferredScope) Get(key string) (lisp.Value, bool) {
	val, ok := s.Scope.Get(key)

	if !ok {
		return lisp.Nil, false
	}

	switch lv := val.(type) {
	case int:
		return lisp.NumberValue(int64(lv)), true
	case int32:
		return lisp.NumberValue(int64(lv)), true
	case int64:
		return lisp.NumberValue(lv), true
	}

	return lisp.StringValue(fmt.Sprintf("%s", val)), true
}

func (s lispInferredScope) Set(key string, v lisp.Value) {
	s.Scope.Set(key, v.Interface())
}

func (s lispInferredScope) Create(key string, v lisp.Value) {
	s.Scope.Set(key, v.Interface())
}

var cDollar = []byte(`$`)

func ExpandVars(s Scope, args string) (string, error) {
	args, err := expandTemplates(s, args)

	if err != nil {
		return "", err
	}

	a := []byte(args)

	var buf bytes.Buffer

	for {
		idx := bytes.Index(a, cDollar)

		if idx == -1 {
			buf.Write(a)
			break
		} else if a[idx+1] == '(' {
			buf.Write(a[:idx])

			in := a[idx+1:]

			fin := findExprClose(in)

			if fin == -1 {
				return "", eUnclosedExpr
			}

			sexp := in[:fin+1]

			ls := lispInferredScope{s}

			val, err := lisp.EvalString(string(sexp), ls)

			if err != nil {
				return "", err
			}

			// fmt.Printf("%s => %s\n", string(sexp), val.Inspect())

			buf.WriteString(val.String())
			a = in[fin+1:]
		} else {
			buf.Write(a[:idx])

			in := a[idx+1:]

			fin := 0

			for fin < len(in) {
				if !varChar(rune(in[fin])) {
					break
				}
				fin++
			}

			if val, ok := s.Get(string(in[:fin])); ok {
				switch val := val.(type) {
				case int64, int:
					buf.WriteString(fmt.Sprintf("%d", val))
				default:
					buf.WriteString(fmt.Sprintf("%s", val))
				}

				a = in[fin:]
			} else {
				return "", fmt.Errorf("Undefined variable: %s", string(in[:fin]))
			}
		}
	}

	return buf.String(), nil
}
