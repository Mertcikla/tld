package dev

import "github.com/spf13/cobra"

func NewDevCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "dev",
		Short: "Developer tools for building and verifying tld",
		Long:  "Developer-only tools for authoring golden fixtures and running conformance checks.",
	}
	c.AddCommand(newFixtureCmd())
	c.AddCommand(newConformanceCmd())
	return c
}
