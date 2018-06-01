package sebak

import (
	"context"
	"time"

	logging "github.com/inconshreveable/log15"
	"github.com/spikeekips/sebak/lib/common"
	"github.com/spikeekips/sebak/lib/network"
	"github.com/spikeekips/sebak/lib/storage"
)

type NodeRunner struct {
	networkID         []byte
	currentNode       sebakcommon.Node
	policy            sebakcommon.VotingThresholdPolicy
	network           sebaknetwork.Network
	consensus         Consensus
	connectionManager *sebaknetwork.ConnectionManager
	storage           *sebakstorage.LevelDBBackend

	handleBallotCheckerFuncs            []sebakcommon.CheckerFunc
	handleMessageFromClientCheckerFuncs []sebakcommon.CheckerFunc

	handleBallotCheckerFuncsContext            context.Context
	handleMessageFromClientCheckerFuncsContext context.Context

	ctx context.Context
	log logging.Logger
}

func NewNodeRunner(
	networkID string,
	currentNode sebakcommon.Node,
	policy sebakcommon.VotingThresholdPolicy,
	network sebaknetwork.Network,
	consensus Consensus,
	storage *sebakstorage.LevelDBBackend,
) *NodeRunner {
	nr := &NodeRunner{
		networkID:   []byte(networkID),
		currentNode: currentNode,
		policy:      policy,
		network:     network,
		consensus:   consensus,
		storage:     storage,
		log:         log.New(logging.Ctx{"node": currentNode.Alias()}),
	}
	nr.ctx = context.WithValue(context.Background(), "currentNode", currentNode)
	nr.ctx = context.WithValue(nr.ctx, "networkID", nr.networkID)

	nr.connectionManager = sebaknetwork.NewConnectionManager(
		nr.currentNode,
		nr.network,
		nr.policy,
		nr.currentNode.GetValidators(),
	)
	nr.network.AddWatcher(nr.connectionManager.ConnectionWatcher)

	nr.SetHandleMessageFromClientCheckerFuncs(nil, DefaultHandleMessageFromClientCheckerFuncs...)
	nr.SetHandleBallotCheckerFuncs(nil, DefaultHandleBallotCheckerFuncs...)
	return nr
}

func (nr *NodeRunner) Ready() {
	nr.network.SetContext(nr.ctx)
	nr.network.Ready()
}

func (nr *NodeRunner) Start() (err error) {
	nr.Ready()

	go nr.handleMessage()
	go nr.ConnectValidators()

	if err = nr.network.Start(); err != nil {
		return
	}

	return
}

func (nr *NodeRunner) Stop() {
	nr.network.Stop()
}

func (nr *NodeRunner) Node() sebakcommon.Node {
	return nr.currentNode
}

func (nr *NodeRunner) NetworkID() []byte {
	return nr.networkID
}

func (nr *NodeRunner) Network() sebaknetwork.Network {
	return nr.network
}

func (nr *NodeRunner) Consensus() Consensus {
	return nr.consensus
}

func (nr *NodeRunner) ConnectionManager() *sebaknetwork.ConnectionManager {
	return nr.connectionManager
}

func (nr *NodeRunner) Storage() *sebakstorage.LevelDBBackend {
	return nr.storage
}

func (nr *NodeRunner) Policy() sebakcommon.VotingThresholdPolicy {
	return nr.policy
}

func (nr *NodeRunner) Log() logging.Logger {
	return nr.log
}

func (nr *NodeRunner) ConnectValidators() {
	ticker := time.NewTicker(time.Millisecond * 5)
	for t := range ticker.C {
		if !nr.network.IsReady() {
			nr.log.Debug("current network is not ready: %v", t)
			continue
		}

		ticker.Stop()
		break
	}
	nr.log.Debug("current node is ready")
	nr.log.Debug("trying to connect to the validators", "validators", nr.currentNode.GetValidators())

	nr.log.Debug("initializing connectionManager for validators")
	nr.connectionManager.Start()
}

var DefaultHandleMessageFromClientCheckerFuncs = []sebakcommon.CheckerFunc{
	CheckNodeRunnerHandleMessageTransactionUnmarshal,
	CheckNodeRunnerHandleMessageHistory,
	CheckNodeRunnerHandleMessageISAACReceiveMessage,
	CheckNodeRunnerHandleMessageSignBallot,
	CheckNodeRunnerHandleMessageBroadcast,
}

var DefaultHandleBallotCheckerFuncs = []sebakcommon.CheckerFunc{
	CheckNodeRunnerHandleBallotIsWellformed,
	CheckNodeRunnerHandleBallotCheckIsNew,
	CheckNodeRunnerHandleBallotReceiveBallot,
	CheckNodeRunnerHandleBallotHistory,
	CheckNodeRunnerHandleBallotStore,
	CheckNodeRunnerHandleBallotIsBroadcastable,
	CheckNodeRunnerHandleBallotVotingHole,
	CheckNodeRunnerHandleBallotBroadcast,
}

func (nr *NodeRunner) SetHandleMessageFromClientCheckerFuncs(ctx context.Context, f ...sebakcommon.CheckerFunc) {
	if len(f) > 0 {
		nr.handleMessageFromClientCheckerFuncs = f
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, "currentNode", nr.currentNode)
	ctx = context.WithValue(ctx, "networkID", nr.networkID)
	nr.handleMessageFromClientCheckerFuncsContext = ctx
}

func (nr *NodeRunner) SetHandleBallotCheckerFuncs(ctx context.Context, f ...sebakcommon.CheckerFunc) {
	if len(f) > 0 {
		nr.handleBallotCheckerFuncs = f
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, "currentNode", nr.currentNode)
	ctx = context.WithValue(ctx, "networkID", nr.networkID)
	nr.handleBallotCheckerFuncsContext = ctx
}

func (nr *NodeRunner) handleMessage() {
	var err error
	for message := range nr.network.ReceiveMessage() {
		switch message.Type {
		case sebaknetwork.ConnectMessage:
			nr.log.Debug("got connect", "message", message.String()[:50])
			if _, err := sebakcommon.NewValidatorFromString(message.Data); err != nil {
				nr.log.Error("invalid validator data was received", "data", message.Data)
				continue
			}
		case sebaknetwork.MessageFromClient:
			nr.log.Debug("got message from client`", "message", message.String()[:50])

			/*
				- TODO check already `IsWellFormed()`
				- TODO check already in BlockTransaction
				- TODO check already in BlockTransactionHistory
			*/

			ctx := nr.handleMessageFromClientCheckerFuncsContext
			if _, err = sebakcommon.Checker(ctx, nr.handleMessageFromClientCheckerFuncs...)(nr, message); err != nil {
				if _, ok := err.(sebakcommon.CheckerErrorStop); ok {
					continue
				}
				nr.log.Error("failed to handle message from client", "error", err)
				nr.closeConsensus(ctx)
				continue
			}
		case sebaknetwork.BallotMessage:
			nr.log.Debug("got ballot", "message", message.String()[:50])
			/*
				- TODO check already `IsWellFormed()`
				- TODO check already in BlockTransaction
				- TODO check already in BlockTransactionHistory
			*/

			ctx := nr.handleBallotCheckerFuncsContext
			if ctx, err = sebakcommon.Checker(ctx, nr.handleBallotCheckerFuncs...)(nr, message); err != nil {
				if _, ok := err.(sebakcommon.CheckerErrorStop); ok {
					nr.closeConsensus(ctx)
					continue
				}
				nr.log.Error("failed to handle ballot", "error", err)

				if err = nr.closeConsensus(ctx); err != nil {
					nr.Log().Error("failed to close consensus", "error", err)
				} else {
					nr.Log().Error("consensus closed")
				}

				continue
			}

			nr.closeConsensus(ctx)
		}
	}
}

func (nr *NodeRunner) closeConsensus(ctx context.Context) (err error) {
	vs, ok := ctx.Value("vs").(VotingStateStaging)
	if !ok {
		return
	}
	if !vs.IsClosed() {
		return
	}

	ballot, ok := ctx.Value("ballot").(Ballot)
	if !ok {
		return
	}
	if err = nr.Consensus().CloseConsensus(ballot); err != nil {
		nr.Log().Error("failed to close consensus", "error", err)
		return
	}

	nr.Log().Debug("consensus closed")
	return
}