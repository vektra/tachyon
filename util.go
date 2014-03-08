package tachyon

import (
	"fmt"
	"github.com/flynn/go-shlex"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"reflect"
	"strconv"
	"strings"
)

func dbg(format string, args ...interface{}) {
	fmt.Printf("[DBG] "+format+"\n", args...)
}

func yamlFile(path string, v interface{}) error {
	data, err := ioutil.ReadFile(path)

	if err != nil {
		return err
	}

	return goyaml.Unmarshal(data, v)
}

func mapToStruct(m map[string]interface{}, tag string, v interface{}) error {
	e := reflect.ValueOf(v).Elem()

	t := e.Type()

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		name := strings.ToLower(f.Name)
		required := false

		parts := strings.Split(f.Tag.Get(tag), ",")

		switch len(parts) {
		case 0:
			// nothing
		case 1:
			name = parts[0]
		case 2:
			name = parts[0]
			switch parts[1] {
			case "required":
				required = true
			default:
				return fmt.Errorf("Unsupported tag flag: %s", parts[1])
			}
		}

		if val, ok := m[name]; ok {
			e.Field(i).Set(reflect.ValueOf(val))
		} else if required {
			return fmt.Errorf("Missing value for %s", f.Name)
		}
	}

	return nil
}

func ParseSimpleMap(s Scope, args string) (Vars, error) {
	args, err := ExpandVars(s, args)

	if err != nil {
		return nil, err
	}

	sm := make(Vars)

	parts, err := shlex.Split(args)

	if err != nil {
		return nil, err
	}

	for _, part := range parts {
		ec := strings.SplitN(part, "=", 2)

		if len(ec) == 2 {
			sm[ec[0]] = Any(inferString(ec[1]))
		} else {
			sm[part] = Any(true)
		}
	}

	return sm, nil
}

func split2(s, sep string) (string, string, bool) {
	parts := strings.SplitN(s, sep, 2)

	if len(parts) == 0 {
		return "", "", false
	} else if len(parts) == 1 {
		return parts[0], "", false
	} else {
		return parts[0], parts[1], true
	}
}

func inferString(s string) interface{} {
	switch strings.ToLower(s) {
	case "true", "yes":
		return true
	case "false", "no":
		return false
	}

	if i, err := strconv.ParseInt(s, 0, 0); err == nil {
		return i
	}

	return s
}

func indentedYAML(v interface{}, indent string) (string, error) {
	str, err := goyaml.Marshal(v)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(str), "\n")

	out := make([]string, len(lines))

	for idx, l := range lines {
		if l == "" {
			out[idx] = l
		} else {
			out[idx] = indent + l
		}
	}

	return strings.Join(out, "\n"), nil
}

func fileExist(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !fi.IsDir()
}
