package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func stringFlag(set *flag.FlagSet, name string, val string, usage string) *string {
	s := set.String(name, val, usage)
	setFromEnv(set, name)
	return s
}

func intFlag(set *flag.FlagSet, name string, val int, usage string) *int {
	s := set.Int(name, val, usage)
	setFromEnv(set, name)
	return s
}

func boolFlag(set *flag.FlagSet, name string, val bool, usage string) *bool {
	s := set.Bool(name, val, usage)
	setFromEnv(set, name)
	return s
}

func setFromEnv(set *flag.FlagSet, name string) {
	val := os.Getenv(strings.ToUpper(strings.Replace(strings.Replace(name, "-", "_", -1), ".", "_", -1)))
	if val == "" {
		return
	}
	if err := set.Set(name, val); err != nil {
		panic(fmt.Sprintf("failed to set value: %s", err.Error()))
	}
}
