package wallet

import (
	"fmt"

	"github.com/spf13/cobra"

	"boscoin.io/sebak/cmd/sebak/common"
	"boscoin.io/sebak/lib"
	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/error"
	"boscoin.io/sebak/lib/network"
)

var (
	CreateAccountCmd *cobra.Command
)

func init() {
	CreateAccountCmd = &cobra.Command{
		Use:   "create-account",
		Short: "Create Account",
		Run: func(c *cobra.Command, args []string) {
			parseWalletFlags()

			client := sebaknetwork.NewHTTP2NetworkClient(endpoint, nil)

			{
				_, err := client.GetAccount(receiverKP.Address())
				if err != sebakerror.ErrorBlockAccountDoesNotExists {
					common.PrintFlagsError(CreateAccountCmd, "receiver account already exists", nil)
				}
			}

			var sourceAccount *sebak.BlockAccount
			{
				b, err := client.GetAccount(senderKP.Address())
				if err == sebakerror.ErrorBlockAccountDoesNotExists {
					common.PrintFlagsError(CreateAccountCmd, "sender account does not exists", nil)
				}

				{
					ba, err := sebak.NewBlockAccountFromByte(b)
					if err != nil {
						common.PrintFlagsError(CreateAccountCmd, "failed to parse received data", nil)
					}
					sourceAccount = ba
				}
			}
			if sourceAccount.GetBalance() < amount {
				common.PrintFlagsError(
					CreateAccountCmd,
					fmt.Sprintf("source does not have enough amount: %v < %v", sourceAccount.GetBalance(), amount),
					nil,
				)
			}

			opb := sebak.NewOperationBodyCreateAccount(receiverKP.Address(), amount)
			op := sebak.Operation{
				H: sebak.OperationHeader{
					Type: sebak.OperationCreateAccount,
				},
				B: opb,
			}

			txBody := sebak.TransactionBody{
				Source:     senderKP.Address(),
				Fee:        sebak.Amount(sebak.BaseFee),
				Checkpoint: sourceAccount.Checkpoint,
				Operations: []sebak.Operation{op},
			}

			tx := sebak.Transaction{
				T: "transaction",
				H: sebak.TransactionHeader{
					Created: sebakcommon.NowISO8601(),
					Hash:    txBody.MakeHashString(),
				},
				B: txBody,
			}
			tx.Sign(senderKP, []byte(flagNetworkID))

			log.Debug("transaction will be sent", "transaction", tx)
			client.SendMessage(tx)
		},
	}

	attachFlags(CreateAccountCmd)
}
