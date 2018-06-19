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
	PaymentCmd *cobra.Command
)

func init() {
	PaymentCmd = &cobra.Command{
		Use:   "payment",
		Short: "Send payment",
		Run: func(c *cobra.Command, args []string) {
			parseWalletFlags()

			client := sebaknetwork.NewHTTP2NetworkClient(endpoint, nil)

			var err error
			var targetAccount *sebak.BlockAccount
			targetAccount, err = getAccount(client, receiverKP.Address())
			if err == sebakerror.ErrorBlockAccountDoesNotExists {
				common.PrintFlagsError(PaymentCmd, "receiver account does not exists", nil)
			} else if err != nil {
				common.PrintFlagsError(PaymentCmd, "failed to parse received data", nil)
			}
			log.Debug("found target account", "account", targetAccount)

			var sourceAccount *sebak.BlockAccount
			sourceAccount, err = getAccount(client, senderKP.Address())
			if err == sebakerror.ErrorBlockAccountDoesNotExists {
				common.PrintFlagsError(PaymentCmd, "sender account does not exists", nil)
			} else if err != nil {
				common.PrintFlagsError(PaymentCmd, "failed to parse received data", nil)
			}
			log.Debug("found source account", "account", sourceAccount)

			if sourceAccount.GetBalance() < amount {
				common.PrintFlagsError(
					PaymentCmd,
					fmt.Sprintf("source does not have enough amount: %v < %v", sourceAccount.GetBalance(), amount),
					nil,
				)
			}

			opb := sebak.NewOperationBodyPayment(receiverKP.Address(), amount)
			op := sebak.Operation{
				H: sebak.OperationHeader{
					Type: sebak.OperationPayment,
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
			// client.SendMessage(tx)
		},
	}

	attachFlags(PaymentCmd)
}
