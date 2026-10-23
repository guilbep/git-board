package gitbugio

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/git-bug/git-bug/entities/identity"
	"github.com/git-bug/git-bug/entity"
	"github.com/git-bug/git-bug/entity/dag"
)

// A release record's operations, as git keeps them forever: git-bug's dag
// stores an operation's type as this number, and its fields by their json
// names. Neither ever changes; a new operation takes the next number.
const (
	releaseCreateOp   dag.OperationType = 1 // create: commit, previous, artifacts, chart
	releaseSetNotesOp dag.OperationType = 2 // set-notes
	releaseCommentOp  dag.OperationType = 3 // comment
	releaseYankOp     dag.OperationType = 4 // yank, with its reason
)

// releaseOp is an operation on a release record.
type releaseOp interface {
	dag.Operation
	apply(r *ReleaseRecord)
}

type releaseCreate struct {
	dag.OpBase
	Commit    string        `json:"commit"`
	Previous  string        `json:"previous"`
	Artifacts []Artifact    `json:"artifacts"`
	Chart     ChartArtifact `json:"chart"`
	Assets    []Asset       `json:"assets,omitempty"` // omitted: the ops of records made before keep their bytes, so their IDs
}

func (op *releaseCreate) Id() entity.Id { return dag.IdOperation(op, &op.OpBase) }

func (op *releaseCreate) Validate() error {
	if op.Commit == "" {
		return errors.New("a release record needs its commit")
	}
	return op.OpBase.Validate(op, releaseCreateOp)
}

func (op *releaseCreate) apply(r *ReleaseRecord) {
	r.Commit, r.Previous, r.Artifacts, r.Chart, r.Assets = op.Commit, op.Previous, op.Artifacts, op.Chart, op.Assets
	r.Author, r.Created = op.Author().Name(), op.Time()
}

type releaseSetNotes struct {
	dag.OpBase
	Notes string `json:"notes"`
}

func (op *releaseSetNotes) Id() entity.Id { return dag.IdOperation(op, &op.OpBase) }

func (op *releaseSetNotes) Validate() error { return op.OpBase.Validate(op, releaseSetNotesOp) }

func (op *releaseSetNotes) apply(r *ReleaseRecord) { r.Notes = op.Notes }

type releaseComment struct {
	dag.OpBase
	Message string `json:"message"`
}

func (op *releaseComment) Id() entity.Id { return dag.IdOperation(op, &op.OpBase) }

func (op *releaseComment) Validate() error {
	if op.Message == "" {
		return errors.New("an empty comment")
	}
	return op.OpBase.Validate(op, releaseCommentOp)
}

func (op *releaseComment) apply(r *ReleaseRecord) {
	r.Comments = append(r.Comments, ReleaseComment{Author: op.Author().Name(), Message: op.Message, Date: op.Time()})
}

type releaseYank struct {
	dag.OpBase
	Reason string `json:"reason"`
}

func (op *releaseYank) Id() entity.Id { return dag.IdOperation(op, &op.OpBase) }

func (op *releaseYank) Validate() error {
	if op.Reason == "" {
		return errors.New("a yank needs its reason")
	}
	return op.OpBase.Validate(op, releaseYankOp)
}

func (op *releaseYank) apply(r *ReleaseRecord) { r.Yanked = op.Reason }

// opBase starts an operation of type t by author, now.
func opBase(t dag.OperationType, author identity.Interface) dag.OpBase {
	return dag.NewOpBase(t, author, time.Now().Unix())
}

// unmarshalReleaseOp reads an operation back from git; dag sets its id and
// author after.
func unmarshalReleaseOp(_ context.Context, raw json.RawMessage, _ entity.Resolvers) (dag.Operation, error) {
	var t struct {
		OperationType dag.OperationType `json:"type"`
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	var op releaseOp
	switch t.OperationType {
	case releaseCreateOp:
		op = &releaseCreate{}
	case releaseSetNotesOp:
		op = &releaseSetNotes{}
	case releaseCommentOp:
		op = &releaseComment{}
	case releaseYankOp:
		op = &releaseYank{}
	default:
		// a later git-board's: kept as written, skipped when compiling
		return dag.NewUnknownOp[dag.Snapshot](raw)
	}
	if err := json.Unmarshal(raw, op); err != nil {
		return nil, err
	}
	return op, nil
}
