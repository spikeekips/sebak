package runner

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	logging "github.com/inconshreveable/log15"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/require"

	"boscoin.io/sebak/lib/ballot"
	"boscoin.io/sebak/lib/block"
	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/consensus"
	"boscoin.io/sebak/lib/consensus/round"
	"boscoin.io/sebak/lib/network"
	"boscoin.io/sebak/lib/node"
	"boscoin.io/sebak/lib/storage"
	"boscoin.io/sebak/lib/transaction"
	"boscoin.io/sebak/lib/transaction/operation"
)

func createNodeRunnerForTestingWithFileStorage(n int, conf common.Config, recv chan struct{}) (*NodeRunner, []*node.LocalNode, string) {
	var ns []*network.MemoryNetwork
	var net *network.MemoryNetwork
	var nodes []*node.LocalNode
	for i := 0; i < n; i++ {
		_, s, v := network.CreateMemoryNetwork(net)
		net = s
		ns = append(ns, s)
		nodes = append(nodes, v)
	}

	for j := 0; j < n; j++ {
		nodes[0].AddValidators(nodes[j].ConvertToValidator())
	}

	localNode := nodes[0]
	policy, _ := consensus.NewDefaultVotingThresholdPolicy(66)

	connectionManager := NewTestConnectionManager(
		localNode,
		ns[0],
		policy,
		recv,
	)

	is, _ := consensus.NewISAAC(networkID, localNode, policy, connectionManager, common.NewConfig())
	is.SetProposerSelector(FixedSelector{localNode.Address()})

	st := &storage.LevelDBBackend{}
	dir, err := ioutil.TempDir("", "sebak-test")
	if err != nil {
		panic(err)
	}

	{
		var err error
		config, _ := storage.NewConfigFromString(fmt.Sprintf("file://%s", dir))

		if err = st.Init(config); err != nil {
			panic(err)
		}

		block.MakeTestBlockchain(st)
	}

	nr, err := NewNodeRunner(string(networkID), localNode, policy, ns[0], is, st, conf)
	if err != nil {
		panic(err)
	}
	nr.isaacStateManager.blockTimeBuffer = 0
	genesisBlock := block.GetGenesis(st)
	nr.Consensus().SetLatestBlock(genesisBlock)

	return nr, nodes, dir
}

func finishBallotWithLimits(numberOfTransactions, numberOfOperations int) error {
	nr, localNodes, dir := createNodeRunnerForTestingWithFileStorage(1, common.NewConfig(), nil)
	defer func() {
		nr.Storage().Close()
		os.RemoveAll(dir)
	}()

	proposerNode := localNodes[0]
	nr.Consensus().SetProposerSelector(FixedSelector{proposerNode.Address()})

	genesisBlock := block.GetGenesis(nr.Storage())
	commonAccount, _ := GetCommonAccount(nr.Storage())
	initialBalance, _ := GetGenesisBalance(nr.Storage())

	var blt *ballot.Ballot
	{
		var txs []transaction.Transaction
		var txHashes []string

		rd := round.Round{
			Number:      0,
			BlockHeight: genesisBlock.Height,
			BlockHash:   genesisBlock.Hash,
			TotalTxs:    genesisBlock.TotalTxs,
		}

		for i := 0; i < numberOfTransactions; i++ {
			kpA, _ := keypair.Random()
			accountA := block.NewBlockAccount(kpA.Address(), common.Amount(common.BaseReserve))
			accountA.MustSave(nr.Storage())

			kpB, _ := keypair.Random()
			tx := transaction.MakeTransactionCreateAccount(kpA, kpB.Address(), common.Amount(1))

			var ops []operation.Operation
			for j := 0; j < numberOfOperations-1; j++ {
				kpC, _ := keypair.Random()

				opb := operation.NewCreateAccount(kpC.Address(), common.Amount(1), "")
				op := operation.Operation{
					H: operation.Header{
						Type: operation.TypeCreateAccount,
					},
					B: opb,
				}
				ops = append(ops, op)
			}
			tx.B.Operations = append(tx.B.Operations, ops...)
			tx.B.SequenceID = accountA.SequenceID
			tx.Sign(kpA, networkID)

			txHashes = append(txHashes, tx.GetHash())
			txs = append(txs, tx)
			nr.TransactionPool.Add(tx)
		}

		blt = ballot.NewBallot(proposerNode.Address(), proposerNode.Address(), rd, txHashes)

		opc, _ := ballot.NewCollectTxFeeFromBallot(*blt, commonAccount.Address, txs...)
		opi, _ := ballot.NewInflationFromBallot(*blt, commonAccount.Address, initialBalance)
		ptx, _ := ballot.NewProposerTransactionFromBallot(*blt, opc, opi)

		blt.SetProposerTransaction(ptx)
		blt.SetVote(ballot.StateINIT, ballot.VotingYES)
		blt.Sign(proposerNode.Keypair(), networkID)
	}

	_, err := finishBallot(
		nr.Storage(),
		*blt,
		nr.TransactionPool,
		nr.Log(),
		nr.Log(),
	)

	return err
}

func benchmarkFinishBallotWithLimits(numberOfTransactions, numberOfOperations int, b *testing.B) {
	SetLogging(logging.LvlError, common.DefaultLogHandler)
	for i := 1; i < b.N+1; i++ {
		finishBallotWithLimits(numberOfTransactions, numberOfOperations)
	}
}

func BenchmarkFinishBallotWithLimits_1_10(b *testing.B) {
	benchmarkFinishBallotWithLimits(1, 10, b)
}

func BenchmarkFinishBallotWithLimits_1_200(b *testing.B) {
	benchmarkFinishBallotWithLimits(1, 200, b)
}

func BenchmarkFinishBallotWithLimits_1_400(b *testing.B) {
	benchmarkFinishBallotWithLimits(1, 400, b)
}

func BenchmarkFinishBallotWithLimits_1_600(b *testing.B) {
	benchmarkFinishBallotWithLimits(1, 600, b)
}

func BenchmarkFinishBallotWithLimits_1_800(b *testing.B) {
	benchmarkFinishBallotWithLimits(1, 800, b)
}

func BenchmarkFinishBallotWithLimits_1_1000(b *testing.B) {
	benchmarkFinishBallotWithLimits(1, 1000, b)
}

func BenchmarkFinishBallotWithLimits_200_10(b *testing.B) {
	benchmarkFinishBallotWithLimits(200, 10, b)
}

func BenchmarkFinishBallotWithLimits_200_200(b *testing.B) {
	benchmarkFinishBallotWithLimits(200, 200, b)
}

func BenchmarkFinishBallotWithLimits_200_400(b *testing.B) {
	benchmarkFinishBallotWithLimits(200, 400, b)
}

func BenchmarkFinishBallotWithLimits_200_600(b *testing.B) {
	benchmarkFinishBallotWithLimits(200, 600, b)
}

func BenchmarkFinishBallotWithLimits_200_800(b *testing.B) {
	benchmarkFinishBallotWithLimits(200, 800, b)
}

func BenchmarkFinishBallotWithLimits_200_1000(b *testing.B) {
	benchmarkFinishBallotWithLimits(200, 1000, b)
}

func BenchmarkFinishBallotWithLimits_400_10(b *testing.B) {
	benchmarkFinishBallotWithLimits(400, 10, b)
}

func BenchmarkFinishBallotWithLimits_400_200(b *testing.B) {
	benchmarkFinishBallotWithLimits(400, 200, b)
}

func BenchmarkFinishBallotWithLimits_400_400(b *testing.B) {
	benchmarkFinishBallotWithLimits(400, 400, b)
}

func BenchmarkFinishBallotWithLimits_400_600(b *testing.B) {
	benchmarkFinishBallotWithLimits(400, 600, b)
}

func BenchmarkFinishBallotWithLimits_400_800(b *testing.B) {
	benchmarkFinishBallotWithLimits(400, 800, b)
}

func BenchmarkFinishBallotWithLimits_400_1000(b *testing.B) {
	benchmarkFinishBallotWithLimits(400, 1000, b)
}

func BenchmarkFinishBallotWithLimits_600_10(b *testing.B) {
	benchmarkFinishBallotWithLimits(600, 10, b)
}

func BenchmarkFinishBallotWithLimits_600_200(b *testing.B) {
	benchmarkFinishBallotWithLimits(600, 200, b)
}

func BenchmarkFinishBallotWithLimits_600_400(b *testing.B) {
	benchmarkFinishBallotWithLimits(600, 400, b)
}

func BenchmarkFinishBallotWithLimits_600_600(b *testing.B) {
	benchmarkFinishBallotWithLimits(600, 600, b)
}

func BenchmarkFinishBallotWithLimits_600_800(b *testing.B) {
	benchmarkFinishBallotWithLimits(600, 800, b)
}

func BenchmarkFinishBallotWithLimits_600_1000(b *testing.B) {
	benchmarkFinishBallotWithLimits(600, 1000, b)
}

func TestBenchmarkFinishBallot(t *testing.T) {
	err := finishBallotWithLimits(100, 100)
	require.Nil(t, err)
}
