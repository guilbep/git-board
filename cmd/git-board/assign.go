package main

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// parseAssignments reads field=value arguments, resolves their aliases and,
// without force, holds them to the config (fields.Check). Every one is
// read before anything is written: one refused, none written.
func parseAssignments(b *store.Backend, cfg *config.Config, args []string, me string, force bool) ([]fields.Assignment, error) {
	var inUse []string
	if !force {
		inUse = b.Labels()
	}
	today := time.Now()
	res := make([]fields.Assignment, 0, len(args))
	var errs []error
	for _, arg := range args {
		a, err := fields.Parse(arg)
		if err == nil {
			a, err = a.Resolve(b, &cfg.Schema, me, today)
		}
		if err == nil && !force {
			err = fields.Check(&cfg.Schema, a, inUse)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		res = append(res, a)
	}
	if err := errors.Join(errs...); err != nil {
		if !force && slices.ContainsFunc(errs, func(e error) bool { return errors.Is(e, fields.ErrUnchecked) }) {
			return nil, errors.Join(err, errors.New("nothing written; --force writes it anyway"))
		}
		return nil, err
	}
	return res, nil
}

// foldAssignments applies the assignments in turn to labels current, and
// returns the labels they leave and what each one did.
func foldAssignments(cfg *config.Config, current []string, as []fields.Assignment) ([]string, []fields.Result, error) {
	labels := slices.Clone(current)
	results := make([]fields.Result, 0, len(as))
	for _, a := range as {
		r, err := fields.Change(&cfg.Schema, labels, a)
		if err != nil {
			return nil, nil, err
		}
		labels = fields.Fold(labels, r)
		results = append(results, r)
	}
	return labels, results, nil
}

// labelDelta is what turns labels from into to.
func labelDelta(from, to []string) (added, removed []string) {
	for _, l := range to {
		if !slices.Contains(from, l) {
			added = append(added, l)
		}
	}
	for _, l := range from {
		if !slices.Contains(to, l) {
			removed = append(removed, l)
		}
	}
	return added, removed
}

// describe says what an assignment did, as set prints it: the label it
// set, -label for a removal, "field cleared".
func describe(a fields.Assignment, r fields.Result) string {
	switch {
	case a.Clears():
		return a.Field + " cleared"
	case a.Op == '-':
		return "-" + r.Label
	}
	return r.Label
}

// statusInStep keeps the issue's state with its status, when the
// assignments set one and [fields.status] has options: the last option
// (close's) closes it, another one reopens it. It answers what it did:
// ", closed", ", reopened" or "".
func statusInStep(b *store.Backend, cfg *config.Config, idPrefix string, as []fields.Assignment, labels []string) (string, error) {
	if !slices.ContainsFunc(as, func(a fields.Assignment) bool { return a.Field == "status" && !a.Clears() && a.Op != '-' }) {
		return "", nil
	}
	options := cfg.Fields["status"].Options
	status := ""
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "status:"); ok {
			status = v
		}
	}
	if !slices.Contains(options, status) {
		return "", nil
	}
	want := "open"
	if status == options[len(options)-1] {
		want = "closed"
	}
	is, err := b.GetIssue(idPrefix)
	if err != nil || is.Status == want {
		return "", err
	}
	if err := b.SetStatus(idPrefix, want); err != nil {
		return "", err
	}
	return map[string]string{"open": ", reopened", "closed": ", closed"}[want], nil
}
