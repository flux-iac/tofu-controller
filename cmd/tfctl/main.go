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
	rootCmd.AddCommand(buildInstallCmd(app))
	rootCmd.AddCommand(buildUninstallCmd(app))
	rootCmd.AddCommand(buildReconcileCmd(app))
	rootCmd.AddCommand(buildSuspendCmd(app))
	rootCmd.AddCommand(buildResumeCmd(app))

	return rootCmd
}

func buildInstallCmd(app *tfctl.CLI) *cobra.Command {
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the tf-controller",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Install(viper.GetString("version"))
		},
	}
	install.Flags().String("version", "", "The version of tf-controller to install.")
	viper.BindPFlag("version", install.Flags().Lookup("version"))
	return install
}

func buildUninstallCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall the tf-controller",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Uninstall()
		},
	}
}

func buildReconcileCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "reconcile NAME",
		Short: "Trigger a reconcile of the provided resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Reconcile(args[0])
		},
	}
}

func buildSuspendCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "suspend NAME",
		Short: "Suspend reconciliation for the provided resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Suspend(args[0])
		},
	}
}

func buildResumeCmd(app *tfctl.CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "resume NAME",
		Short: "Resume reconciliation for the provided resource",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Resume(args[0])
		},
	}
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
