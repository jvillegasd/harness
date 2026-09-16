package scenarios

import (
	"fmt"
	"strconv"
	"strings"
)

func newFlags() *flagSet {
	return &flagSet{values: map[string]*string{}, bools: map[string]*bool{}, ints: map[string]*int{}, set: map[string]bool{}}
}

// flagSet is a minimal parser for the subcommand flags, which all take a value or are boolean.
type flagSet struct {
	values map[string]*string
	bools  map[string]*bool
	ints   map[string]*int
	set    map[string]bool
}

func (f *flagSet) String(name, def, _ string) *string {
	value := def
	f.values[name] = &value
	return &value
}

func (f *flagSet) Bool(name string, def bool, _ string) *bool {
	value := def
	f.bools[name] = &value
	return &value
}

func (f *flagSet) Int(name string, def int, _ string) *int {
	value := def
	f.ints[name] = &value
	return &value
}

func (f *flagSet) Parse(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return fmt.Errorf("unexpected argument %q", arg)
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if b, ok := f.bools[name]; ok {
			if hasValue {
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("--%s: %v", name, err)
				}
				*b = parsed
			} else {
				*b = true
			}
			f.set[name] = true
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return fmt.Errorf("--%s needs a value", name)
			}
			i++
			value = args[i]
		}
		if target, ok := f.values[name]; ok {
			*target = value
			f.set[name] = true
			continue
		}
		if target, ok := f.ints[name]; ok {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("--%s: %v", name, err)
			}
			*target = parsed
			f.set[name] = true
			continue
		}
		return fmt.Errorf("unknown flag --%s", name)
	}
	return nil
}

func isFlagSet(f *flagSet, name string) bool { return f.set[name] }
