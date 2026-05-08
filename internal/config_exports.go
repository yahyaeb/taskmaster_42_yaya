package internal

import (
	"taskmaster/internal/config"
)

// Re-export config types for backward compatibility with existing tests.
type (
	ConfigSpec    = config.ConfigSpec
	ConfigLoader  = config.Loader
	YAMLConfigLoader = config.YAMLLoader
)

// Re-export functions
var (
	FormatInstanceName = config.FormatInstanceName
)
