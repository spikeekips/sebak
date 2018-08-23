package sebak

import (
	"testing"
	"time"

	"github.com/btcsuite/btcutil/base58"
	"github.com/stellar/go/keypair"
	"github.com/stretchr/testify/require"

	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/error"
	"boscoin.io/sebak/lib/node"
)

func TestBallotBadConfirmedTime(t *testing.T) {
	kp, _ := keypair.Random()
	endpoint, _ := sebakcommon.NewEndpointFromString("https://localhost:1000")
	node, _ := sebaknode.NewLocalNode(kp, endpoint, "")

	round := Round{Number: 0, BlockHeight: 0, BlockHash: "", TotalTxs: 0}

	updateBallot := func(ballot *Ballot) {
		ballot.H.Hash = ballot.B.MakeHashString()
		signature, _ := sebakcommon.MakeSignature(kp, networkID, ballot.H.Hash)
		ballot.H.Signature = base58.Encode(signature)
	}

	{
		ballot := NewBallot(node, round, []string{})
		ballot.Sign(kp, networkID)

		err := ballot.IsWellFormed(networkID)
		require.Nil(t, err)
	}

	{ // bad `Ballot.B.Confirmed` time; too ahead
		ballot := NewBallot(node, round, []string{})
		ballot.Sign(kp, networkID)

		newConfirmed := time.Now().Add(time.Duration(2) * BallotConfirmedTimeAllowDuration)
		ballot.B.Confirmed = sebakcommon.FormatISO8601(newConfirmed)
		updateBallot(ballot)

		err := ballot.IsWellFormed(networkID)
		require.Error(t, err, sebakerror.ErrorMessageHasIncorrectTime)
	}

	{ // bad `Ballot.B.Confirmed` time; too behind
		ballot := NewBallot(node, round, []string{})
		ballot.Sign(kp, networkID)

		newConfirmed := time.Now().Add(time.Duration(-2) * BallotConfirmedTimeAllowDuration)
		ballot.B.Confirmed = sebakcommon.FormatISO8601(newConfirmed)
		updateBallot(ballot)

		err := ballot.IsWellFormed(networkID)
		require.Error(t, err, sebakerror.ErrorMessageHasIncorrectTime)
	}

	{ // bad `Ballot.B.Proposed.Confirmed` time; too ahead
		ballot := NewBallot(node, round, []string{})
		ballot.Sign(kp, networkID)

		newConfirmed := time.Now().Add(time.Duration(2) * BallotConfirmedTimeAllowDuration)
		ballot.B.Proposed.Confirmed = sebakcommon.FormatISO8601(newConfirmed)
		updateBallot(ballot)

		err := ballot.IsWellFormed(networkID)
		require.Error(t, err, sebakerror.ErrorMessageHasIncorrectTime)
	}

	{ // bad `Ballot.B.Proposed.Confirmed` time; too behind
		ballot := NewBallot(node, round, []string{})
		ballot.Sign(kp, networkID)

		newConfirmed := time.Now().Add(time.Duration(-2) * BallotConfirmedTimeAllowDuration)
		ballot.B.Proposed.Confirmed = sebakcommon.FormatISO8601(newConfirmed)
		updateBallot(ballot)

		err := ballot.IsWellFormed(networkID)
		require.Error(t, err, sebakerror.ErrorMessageHasIncorrectTime)
	}
}
