package wallet

import (
	"errors"
	"os"

	logging "github.com/inconshreveable/log15"
	isatty "github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/stellar/go/keypair"

	"boscoin.io/sebak/cmd/sebak/common"
	"boscoin.io/sebak/lib"
	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/network"
)

var (
	flagNetworkID      string
	flagSecretSeed     string
	flagReceiver       string
	flagAmount         uint
	flagEndpointString string
	flagLogLevel       string = "debug"
)

var (
	endpoint   *sebakcommon.Endpoint
	senderKP   *keypair.Full
	receiverKP keypair.KP
	amount     sebak.Amount
	log        logging.Logger
	logLevel   logging.Lvl
)

func parseWalletFlags() {
	var err error

	if logLevel, err = logging.LvlFromString(flagLogLevel); err != nil {
		common.PrintFlagsError(CreateAccountCmd, "--log-level", err)
	}

	var logHandler logging.Handler

	var formatter logging.Format
	if isatty.IsTerminal(os.Stdout.Fd()) {
		formatter = logging.TerminalFormat()
	} else {
		formatter = logging.JsonFormatEx(false, true)
	}
	logHandler = logging.StreamHandler(os.Stdout, formatter)

	log = logging.New("module", "main")
	log.SetHandler(logging.LvlFilterHandler(logLevel, logHandler))
	sebak.SetLogging(logLevel, logHandler)
	sebaknetwork.SetLogging(logLevel, logHandler)

	if len(flagNetworkID) < 1 {
		common.PrintFlagsError(CreateAccountCmd, "--network-id", errors.New("-network-id must be given"))
	}

	if p, err := sebakcommon.ParseNodeEndpoint(flagEndpointString); err != nil {
		common.PrintFlagsError(CreateAccountCmd, "--endpoint", err)
	} else {
		endpoint = p
	}

	var parsedKP keypair.KP
	parsedKP, err = keypair.Parse(flagSecretSeed)
	if err != nil {
		common.PrintFlagsError(CreateAccountCmd, "--secret-seed", err)
	} else {
		senderKP = parsedKP.(*keypair.Full)
	}

	parsedKP, err = keypair.Parse(flagReceiver)
	if err != nil {
		common.PrintFlagsError(CreateAccountCmd, "--receiver", err)
	} else {
		receiverKP = parsedKP
	}

	amount = sebak.Amount(flagAmount)
	amount.Invariant()
}

func getAccount(client *sebaknetwork.HTTP2NetworkClient, address string) (ba *sebak.BlockAccount, err error) {
	var b []byte
	b, err = client.GetAccount(address)
	if err != nil {
		return
	}

	ba, err = sebak.NewBlockAccountFromByte(b)
	if err != nil {
		return
	}

	return
}

func attachFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flagNetworkID, "network-id", "", "network id")
	cmd.Flags().StringVar(&flagEndpointString, "endpoint", "https://localhost:12345", "node endpoint")
	cmd.Flags().StringVar(&flagSecretSeed, "secret-seed", "", "sender's secret seed")
	cmd.Flags().StringVar(&flagReceiver, "receiver", "", "receiver's public address")
	cmd.Flags().UintVar(&flagAmount, "amount", 0, "amount to send")
	cmd.Flags().StringVar(&flagLogLevel, "log-level", flagLogLevel, "log level, {crit, error, warn, info, debug}")

	cmd.MarkFlagRequired("network-id")
	cmd.MarkFlagRequired("secret-seed")
	cmd.MarkFlagRequired("receiver")
	cmd.MarkFlagRequired("amount")
}
