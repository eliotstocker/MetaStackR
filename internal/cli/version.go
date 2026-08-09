package cli

import (
	"fmt"
	"os"

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
			useCmd := "git meta"
			if len(os.Args) > 0 && (os.Args[0] == "./git-meta" || os.Args[0] == "git-meta") && os.Getenv("GIT_EXEC_PATH") == "" {
				useCmd = "git-meta"
			}
			fmt.Printf("%s version %s\n", useCmd, Version)
		},
	}
}
