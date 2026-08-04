package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "v1.0.0"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the semantic version of git-meta",
		Run: func(cmd *cobra.Command, args []string) {
			if jsonOutput {
				printJSON(true, "", map[string]string{
					"version": Version,
				})
				return
			}
			PrintBanner()
			fmt.Printf("git-meta version %s\n", Version)
		},
	}
}
