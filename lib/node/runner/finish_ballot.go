package runner

import (
	"encoding/json"

	logging "github.com/inconshreveable/log15"

	"boscoin.io/sebak/lib/ballot"
	"boscoin.io/sebak/lib/block"
	"boscoin.io/sebak/lib/error"
	"boscoin.io/sebak/lib/storage"
	"boscoin.io/sebak/lib/transaction"
	"boscoin.io/sebak/lib/transaction/operation"
)

func finishBallot(st *storage.LevelDBBackend, b ballot.Ballot, transactionPool *transaction.Pool, log, infoLog logging.Logger) (*block.Block, error) {
	var err error

	transactions := map[string]transaction.Transaction{}
	for _, hash := range b.B.Proposed.Transactions {
		tx, found := transactionPool.Get(hash)
		if !found {
			return nil, errors.ErrorTransactionNotFound
		}
		transactions[hash] = tx
	}

	blk := block.NewBlock(
		b.Proposer(),
		b.Round(),
		b.ProposerTransaction().GetHash(),
		b.Transactions(),
		b.ProposerConfirmed(),
	)

	batch := storage.NewBatch()
	defer func() {
		batch = nil
	}()

	if err = blk.Batch(st, batch); err != nil {
		log.Error("failed to create new block", "block", blk, "error", err)
		return nil, err
	}

	log.Debug("NewBlock created", "block", *blk)
	infoLog.Info("NewBlock created",
		"height", blk.Height,
		"round", blk.Round.Number,
		"timestamp", blk.Header.Timestamp,
		"total-txs", blk.Round.TotalTxs,
		"proposer", blk.Proposer,
	)

	for _, hash := range b.B.Proposed.Transactions {
		tx := transactions[hash]
		raw, _ := json.Marshal(tx)

		bt := block.NewBlockTransactionFromTransaction(blk.Hash, blk.Height, blk.Confirmed, tx, raw)
		if err = bt.Batch(st, batch); err != nil {
			log.Error("failed to create new BlockTransaction", "block", blk, "bt", bt, "error", err)
			return nil, err
		}
		for _, op := range tx.B.Operations {
			if err = finishOperation(st, tx.B.Source, op, batch, log); err != nil {
				log.Error("failed to finish operation", "block", blk, "bt", bt, "op", op, "error", err)
				return nil, err
			}
		}

		var baSource *block.BlockAccount
		if ac, found := batch.Get(tx.B.Source); found {
			baSource = ac.(*block.BlockAccount)
		} else {
			if baSource, err = block.GetBlockAccount(st, tx.B.Source); err != nil {
				err = errors.ErrorBlockAccountDoesNotExists
				return nil, err
			}
		}

		if err = baSource.Withdraw(tx.TotalAmount(true)); err != nil {
			return nil, err
		}
		batch.Set(tx.B.Source, baSource)
	}

	if err = finishProposerTransaction(st, *blk, b.ProposerTransaction(), batch, log); err != nil {
		log.Error("failed to finish proposer transaction", "block", blk, "ptx", b.ProposerTransaction(), "error", err)
		return nil, err
	}

	var ac *block.BlockAccount
	for instance := range batch.RangeInstance() {
		var ok bool
		if ac, ok = instance.(*block.BlockAccount); !ok {
			continue
		}

		ac.Batch(st, batch)
	}

	if err = batch.Write(st); err != nil {
		return nil, err
	}

	return blk, nil
}

// finishOperation do finish the task after consensus by the type of each operation.
func finishOperation(st *storage.LevelDBBackend, source string, op operation.Operation, batch *storage.Batch, log logging.Logger) (err error) {
	switch op.H.Type {
	case operation.TypeCreateAccount:
		pop, ok := op.B.(operation.CreateAccount)
		if !ok {
			return errors.ErrorUnknownOperationType
		}
		return finishCreateAccount(st, source, pop, batch, log)
	case operation.TypePayment:
		pop, ok := op.B.(operation.Payment)
		if !ok {
			return errors.ErrorUnknownOperationType
		}
		return finishPayment(st, source, pop, batch, log)
	case operation.TypeCongressVoting, operation.TypeCongressVotingResult:
		//Nothing to do
		return
	case operation.TypeUnfreezingRequest:
		pop, ok := op.B.(operation.UnfreezeRequest)
		if !ok {
			return errors.ErrorUnknownOperationType
		}
		return finishUnfreezeRequest(st, source, pop, batch, log)
	default:
		err = errors.ErrorUnknownOperationType
		return
	}
}

func finishCreateAccount(st *storage.LevelDBBackend, source string, op operation.CreateAccount, batch *storage.Batch, log logging.Logger) (err error) {

	var baSource, baTarget *block.BlockAccount
	if ac, found := batch.Get(source); found {
		baSource = ac.(*block.BlockAccount)
	} else {
		if baSource, err = block.GetBlockAccount(st, source); err != nil {
			err = errors.ErrorBlockAccountDoesNotExists
			return
		}
	}
	if ac, found := batch.Get(op.TargetAddress()); found {
		baTarget = ac.(*block.BlockAccount)
	} else {
		if baTarget, err = block.GetBlockAccount(st, op.TargetAddress()); err == nil {
			err = errors.ErrorBlockAccountAlreadyExists
			return
		} else {
			err = nil
		}
	}

	baTarget = block.NewBlockAccountLinked(
		op.TargetAddress(),
		op.GetAmount(),
		op.Linked,
	)

	batch.Set(source, baSource)
	batch.Set(op.TargetAddress(), baTarget)

	log.Debug("new account created", "source", baSource, "target", baTarget)

	return
}

func finishPayment(st *storage.LevelDBBackend, source string, op operation.Payment, batch *storage.Batch, log logging.Logger) (err error) {

	var baSource, baTarget *block.BlockAccount
	if ac, found := batch.Get(source); found {
		baSource = ac.(*block.BlockAccount)
	} else {
		if baSource, err = block.GetBlockAccount(st, source); err != nil {
			err = errors.ErrorBlockAccountDoesNotExists
			return
		}
	}
	if ac, found := batch.Get(op.TargetAddress()); found {
		baTarget = ac.(*block.BlockAccount)
	} else {
		if baTarget, err = block.GetBlockAccount(st, op.TargetAddress()); err != nil {
			err = errors.ErrorBlockAccountDoesNotExists
			return
		}
	}

	if err = baTarget.Deposit(op.GetAmount()); err != nil {
		return
	}

	batch.Set(source, baSource)
	batch.Set(op.TargetAddress(), baTarget)

	log.Debug("payment done", "source", baSource, "target", baTarget, "amount", op.GetAmount())

	return
}

func finishProposerTransaction(st *storage.LevelDBBackend, blk block.Block, ptx ballot.ProposerTransaction, batch *storage.Batch, log logging.Logger) (err error) {
	{
		var opb operation.CollectTxFee
		if opb, err = ptx.CollectTxFee(); err != nil {
			return
		}
		if err = finishCollectTxFee(st, opb, batch, log); err != nil {
			return
		}
	}

	{
		var opb operation.Inflation
		if opb, err = ptx.Inflation(); err != nil {
			return
		}
		if err = finishInflation(st, opb, batch, log); err != nil {
			return
		}
	}

	raw, _ := json.Marshal(ptx.Transaction)
	bt := block.NewBlockTransactionFromTransaction(blk.Hash, blk.Height, blk.Confirmed, ptx.Transaction, raw)
	if err = bt.Batch(st, batch); err != nil {
		return
	}

	return
}

func finishCollectTxFee(st *storage.LevelDBBackend, opb operation.CollectTxFee, batch *storage.Batch, log logging.Logger) (err error) {
	if opb.Amount < 1 {
		return
	}

	var commonAccount *block.BlockAccount
	if ac, found := batch.Get(opb.TargetAddress()); found {
		commonAccount = ac.(*block.BlockAccount)
	} else {
		if commonAccount, err = block.GetBlockAccount(st, opb.TargetAddress()); err != nil {
			return
		}
	}

	if err = commonAccount.Deposit(opb.GetAmount()); err != nil {
		return
	}

	batch.Set(opb.TargetAddress(), commonAccount)

	return
}

func finishInflation(st *storage.LevelDBBackend, opb operation.Inflation, batch *storage.Batch, log logging.Logger) (err error) {
	if opb.Amount < 1 {
		return
	}

	var commonAccount *block.BlockAccount
	if ac, found := batch.Get(opb.TargetAddress()); found {
		commonAccount = ac.(*block.BlockAccount)
	} else {
		if commonAccount, err = block.GetBlockAccount(st, opb.TargetAddress()); err != nil {
			return
		}
	}

	if err = commonAccount.Deposit(opb.GetAmount()); err != nil {
		return
	}

	batch.Set(opb.TargetAddress(), commonAccount)

	return
}

func finishUnfreezeRequest(st *storage.LevelDBBackend, source string, opb operation.UnfreezeRequest, batch *storage.Batch, log logging.Logger) (err error) {
	log.Debug("UnfreezeRequest done")

	return
}
