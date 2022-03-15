package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/weaveworks/tf-controller/tfctl"
)

func main() {
	cmd := run()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run() *cobra.Command {
	app := tfctl.New()

	rootCmd := &cobra.Command{
		Use:           "tfctl",
		SilenceErrors: false,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return app.Init(viper.GetString("kubeconfig"), viper.GetString("namespace"), viper.GetString("terraform"))
		},
	}

	rootCmd.PersistentFlags().String("kubeconfig", "", "Path to the kubeconfig file to use for CLI requests.")
	rootCmd.PersistentFlags().StringP("namespace", "n", "default", "The kubernetes namespace to use for CLI requests.")
	rootCmd.PersistentFlags().String("terraform", "/usr/bin/terraform", "The location of the terraform binary.")

	viper.BindPFlag("kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	viper.BindPFlag("namespace", rootCmd.PersistentFlags().Lookup("namespace"))
	viper.BindPFlag("terraform", rootCmd.PersistentFlags().Lookup("terraform"))

	viper.BindEnv("kubeconfig")

	rootCmd.AddCommand(buildPlanCmd(app))

	return rootCmd
}

func buildPlanCmd(app *tfctl.CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan a Terraform configuration",
	}

	cmd.AddCommand(buildPlanShowCmd(app))

	return cmd
}

func buildPlanShowCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "show NAME",
		Short: "Show a Terraform plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.ShowPlan(os.Stdout, args[0])
		},
	}
}
