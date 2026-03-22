// Package main is the entry point for octometrics.
//
//go:generate go run github.com/vektra/mockery/v3@v3.7.0
package main

import "github.com/kalverra/octometrics/cmd"

func main() {
	cmd.Execute()
}
