//go:build tools
// +build tools

// Package tools is used to manage tool dependencies via go mod.
//
// This pattern allows us to:
// 1. Track tool versions in go.mod
// 2. Install tools via 'go install'
// 3. Ensure consistent versions across developers
package tools

import (
	_ "github.com/dmarkham/enumer"
	_ "go.uber.org/mock/mockgen"
)
