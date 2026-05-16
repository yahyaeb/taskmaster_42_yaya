package app

import "taskmaster/internal/config"

// ConfigLoader loads program specs from a path (e.g. YAML).
type ConfigLoader interface {
	Load(path string) ([]config.ConfigSpec, error)
}
