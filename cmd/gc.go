/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// gcCmd represents the gc command
var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Garbage collect the SQLite database",
	Long: `Delete all posts that are older than 30 days from the SQLite database.

Can be run as a cron job to keep the database size down.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("gc called")
	},
}

func init() {
	rootCmd.AddCommand(gcCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// gcCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// gcCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
