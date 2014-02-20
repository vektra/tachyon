package tachyon

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"reflect"
	"strings"
)

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
