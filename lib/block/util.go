package block

import (
	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/error"
	"boscoin.io/sebak/lib/storage"
	"boscoin.io/sebak/lib/transaction"
	"boscoin.io/sebak/lib/transaction/operation"
)

func getGenesisTransaction(st *storage.LevelDBBackend) (bt BlockTransaction, err error) {
	bk := GetGenesis(st)
	if len(bk.Transactions) < 1 {
		err = errors.ErrorWrongBlockFound
		return
	}

	if bt, err = GetBlockTransaction(st, bk.Transactions[0]); err != nil {
		return
	}

	if len(bt.Transaction().B.Operations) != 2 {
		err = errors.ErrorWrongBlockFound
		return
	}

	return
}

func getGenesisAccount(st *storage.LevelDBBackend, operationIndex int) (account *BlockAccount, err error) {
	var bt BlockTransaction
	if bt, err = getGenesisTransaction(st); err != nil {
		return
	}

	opbp := bt.Transaction().B.Operations[operationIndex].B.(operation.Payable)

	if account, err = GetBlockAccount(st, opbp.TargetAddress()); err != nil {
		return
	}

	return
}

func GetGenesisAccount(st *storage.LevelDBBackend) (account *BlockAccount, err error) {
	return getGenesisAccount(st, 0)
}

func GetCommonAccount(st *storage.LevelDBBackend) (account *BlockAccount, err error) {
	return getGenesisAccount(st, 1)
}

func GetGenesisBalance(st *storage.LevelDBBackend) (balance common.Amount, err error) {
	var bt BlockTransaction
	if bt, err = getGenesisTransaction(st); err != nil {
		return
	}

	var sbt transaction.Transaction
	if err = common.DecodeJSONValue(bt.Message, &sbt); err != nil {
		return
	}
	opbp := sbt.B.Operations[0].B.(operation.Payable)
	balance = opbp.GetAmount()

	return
}
