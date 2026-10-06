// Package main is the entry point for octometrics.
//
//go:generate go run github.com/vektra/mockery/v3@v3.8.0
package main

import (
	_ "embed"

	"github.com/kalverra/octometrics/cmd"
)

//go:embed SKILL.md
var skillContent string

func init() {
	cmd.SetSkillContent(skillContent)
}

func main() {
	cmd.Execute()
}
