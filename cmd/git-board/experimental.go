package main

import "github.com/guilbep/git-board/internal/config"

// needExperimental refuses a command of a work-in-progress feature that is
// off in dir's board ([experimental] features, git config
// board.experimental); such a command is Hidden from the help too.
func needExperimental(dir, feature string) error {
	cfg, err := config.Load(config.PathIn(dir))
	if err != nil {
		return err
	}
	if !cfg.Experimental(feature) {
		return config.ExperimentalOff(feature)
	}
	return nil
}
