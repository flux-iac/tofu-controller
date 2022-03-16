package utils

import "strings"

func EnvMap(environ []string) map[string]string {
	env := map[string]string{}
	for _, ev := range environ {
		parts := strings.SplitN(ev, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := parts[0]
		v := ""
		if len(parts) == 2 {
			v = parts[1]
		}
		env[k] = v
	}
	return env
}
