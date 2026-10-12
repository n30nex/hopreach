package config

import "fmt"

type MeshMapperConfig struct {
	Enabled           bool    `yaml:"enabled"`
	APIKeyFile        string  `yaml:"api_key_file"`
	StateDir          string  `yaml:"state_dir"`
	IntervalHours     float64 `yaml:"interval_hours"`
	MaxCellsPerRegion int     `yaml:"max_cells_per_region"`
}

func (c Config) validateMeshMapper() error {
	if !c.MeshMapper.Enabled {
		return nil
	}
	if len(c.Regions.Entries) == 0 || c.Region.BoundaryPath == "" {
		return fmt.Errorf("MeshMapper requires configured Canadian IATA regions and a country boundary")
	}
	if !finite(c.MeshMapper.IntervalHours) || c.MeshMapper.IntervalHours < 12 {
		return fmt.Errorf("global MeshMapper interval_hours must be at least 12")
	}
	if c.MeshMapper.StateDir == "" || c.MeshMapper.MaxCellsPerRegion < 1 || c.MeshMapper.MaxCellsPerRegion > 100000 {
		return fmt.Errorf("MeshMapper requires private state_dir and max_cells_per_region in [1,100000]")
	}
	return nil
}
