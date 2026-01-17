//go:build !linux

package container

import "errors"

var ErrNotSupported = errors.New("container isolation requires Linux")

type Executor struct{}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) SetVerbose(v bool)	{}

func (e *Executor) IsSupported() bool {
	return false
}

func (e *Executor) RequiresRoot() bool {
	return true
}

func IsChild() bool {
	return false
}

func (e *Executor) Run(fn func() error) error {
	return ErrNotSupported
}
