package cmd

import (
	"github.com/spf13/cobra"

	"boscoin.io/sebak/cmd/sebak/cmd/wallet"
)

var (
	walletCmd *cobra.Command
)

func init() {
	walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "wallet",
		Run: func(c *cobra.Command, args []string) {
			if len(args) < 1 {
				c.Usage()
			}
		},
	}

	walletCmd.AddCommand(wallet.CreateAccountCmd)
	walletCmd.AddCommand(wallet.PaymentCmd)
	rootCmd.AddCommand(walletCmd)
}
