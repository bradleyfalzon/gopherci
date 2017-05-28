package analyser

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
)

// RefReader returns the base reference of a repository.
type RefReader interface {
	Base(context.Context, Executer) (string, error)
}

// MergeBase is a RefReader for handling pull requests by using git's merge-base
// tool to find the common ancestor between HEAD and FETCH_HEAD. It expects
// head to already be checked out, and base to be fetched with full history.
type MergeBase struct{}

var _ RefReader = &MergeBase{}

// Base implements the RefReader interface.
func (b *MergeBase) Base(ctx context.Context, exec Executer) (string, error) {
	args := []string{"git", "merge-base", "FETCH_HEAD", "HEAD"}
	out, err := exec.Execute(ctx, args)
	if err != nil {
		return "", errors.WithMessage(err, fmt.Sprintf("could not execute %v: %q", args, out))
	}
	return string(bytes.TrimSpace(out)), nil
}

// FixedRef is a RefReader for handling events where we know the base ref and
// can just return the string.
type FixedRef struct {
	BaseRef string
}

var _ RefReader = &FixedRef{}

// Base implements the RefReader interface.
func (b *FixedRef) Base(ctx context.Context, exec Executer) (string, error) {
	return b.BaseRef, nil
}
