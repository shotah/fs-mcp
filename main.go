package main

import (
	"fmt"
	"log"
	"os"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/shotah/fs-mcp/server"
	"github.com/shotah/fs-mcp/tools"
	"github.com/spf13/cobra"
)

func main() {
	var rootPath, tier string
	cmd := &cobra.Command{
		Use:   "fs-mcp",
		Short: "Workspace file tools over MCP stdio",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := tools.CleanRoot(rootPath)
			if err != nil {
				return err
			}
			s := server.New()
			n, err := tools.Register(s, root, tier)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "fs-mcp: published %d tools (tier=%s)\n", n, tier)
			return mcpserver.ServeStdio(s, mcpserver.WithErrorLogger(log.New(os.Stderr, "", log.LstdFlags)))
		},
	}
	cmd.Flags().StringVar(&rootPath, "root", "", "workspace jail (required)")
	cmd.Flags().StringVar(&tier, "tool-tier", tools.TierCore, "core or write")
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
