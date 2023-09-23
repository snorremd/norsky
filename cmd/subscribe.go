/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// logCmd represents the log command
var logCmd = &cobra.Command{
	Use:   "subscribe",
	Short: "Log all norwegian posts to the command line",
	Long: `Subscribe to the Bluesky firehose and log all posts written in
Norwegian bokmål, nynorsk and sami to the command line.

Can be used if you want to collect all posts written in Norwegian by
passing the output to a file or another application.

Returns each post as a JSON object on a single line. Use a tool like jq to process
the output.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("log called")
	},
}

func init() {
	rootCmd.AddCommand(logCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// logCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// logCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
