package main

import (
	"errors"
	"fmt"
	"log"

	"hopreach/internal/config"
)

// runConfigured reuses the established pipeline, one region at a time under
// main's process lock. No visitor request starts terrain work, and each region
// has independent freshness checks, progress and last successful output.
func runConfigured(c config.Config, force, all bool, runOne func(appConfig) error) error {
	if len(c.Regions.Entries) == 0 {
		cfg := toAppConfig(c)
		cfg.forceRecompute = force || all
		cfg.forceAllTiers = all
		return runOne(cfg)
	}
	var failures []error
	for _, r := range c.Regions.Entries {
		selected, err := c.ForIATA(r.IATA)
		if err != nil {
			return err
		}
		cfg := toAppConfig(selected)
		cfg.forceRecompute = force || all
		cfg.forceAllTiers = all
		log.Printf("regional coverage: %s (%s)", r.IATA, r.Name)
		if err := runOne(cfg); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", r.IATA, err))
		}
	}
	return errors.Join(failures...)
}
