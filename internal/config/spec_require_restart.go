package config

import (
	"maps"
	"slices"
)

func SpecsRequireRestart(oldSpec, newSpec *ConfigSpec) bool {
	if oldSpec == nil || newSpec == nil {
		return true
	}
	if oldSpec.Cmd != newSpec.Cmd ||
		oldSpec.Program != newSpec.Program ||
		oldSpec.Workingdir != newSpec.Workingdir ||
		oldSpec.Umask != newSpec.Umask ||
		oldSpec.Stdout != newSpec.Stdout ||
		oldSpec.Stderr != newSpec.Stderr ||
		oldSpec.Stopsignal != newSpec.Stopsignal ||
		oldSpec.Stoptime != newSpec.Stoptime ||
		oldSpec.Starttime != newSpec.Starttime ||
		oldSpec.Startretries != newSpec.Startretries {
		return true
	}
	if !slices.Equal(oldSpec.Exitcodes, newSpec.Exitcodes) {
		return true
	}
	return !maps.Equal(oldSpec.Env, newSpec.Env)
}
