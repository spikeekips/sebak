package block

import (
	"encoding/json"
	"fmt"

	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/common/observer"
	"boscoin.io/sebak/lib/error"
	"boscoin.io/sebak/lib/storage"
	"boscoin.io/sebak/lib/transaction"
	"boscoin.io/sebak/lib/transaction/operation"
)

// BlockTransaction is `Transaction` data for block. the storage should support,
//  * find by `Hash`
//
//  * get list by `SequenceID` and created order
//  * get list by `Source` and created order
//  * get list by `Confirmed` order
//  * get list by `Account` and created order
//  * get list by `Block` and created order

// TODO(BlockTransaction): support counting

type BlockTransaction struct {
	Hash  string `json:"hash"`
	Block string/* `Block.Hash` */ `json:"block"`

	Message []byte `json:"message"`

	transaction transaction.Transaction
	isSaved     bool
	confirmed   string
	blockHeight uint64
}

func NewBlockTransactionFromTransaction(blockHash string, blockHeight uint64, confirmed string, tx transaction.Transaction, message []byte) BlockTransaction {
	/*
		var opHashes []string
		for _, op := range tx.B.Operations {
			opHashes = append(opHashes, NewBlockOperationKey(op.MakeHashString(), tx.GetHash()))
		}
	*/

	return BlockTransaction{
		Hash:        tx.H.Hash,
		Block:       blockHash,
		Message:     message,
		transaction: tx,
		blockHeight: blockHeight,
		confirmed:   confirmed,
	}
}

func (bt *BlockTransaction) LoadTransaction() (err error) {
	var tx transaction.Transaction
	if err = common.DecodeJSONValue(bt.Message, &tx); err != nil {
		return
	}

	bt.transaction = tx
	return
}

func (bt BlockTransaction) NewBlockTransactionKeySource() string {
	return fmt.Sprintf(
		"%s%s%s%s",
		GetBlockTransactionKeyPrefixSource(bt.transaction.Source()),
		common.EncodeUint64ToByteSlice(bt.blockHeight),
		common.EncodeUint64ToByteSlice(bt.transaction.B.SequenceID),
		common.GetUniqueIDFromUUID(),
	)
}

func (bt BlockTransaction) NewBlockTransactionKeyConfirmed() string {
	return fmt.Sprintf(
		"%s%s",
		GetBlockTransactionKeyPrefixConfirmed(bt.confirmed),
		common.GetUniqueIDFromUUID(),
	)
}

func (bt BlockTransaction) NewBlockTransactionKeyByAccount(accountAddress string) string {
	return fmt.Sprintf(
		"%s%s%s%s",
		GetBlockTransactionKeyPrefixAccount(accountAddress),
		common.EncodeUint64ToByteSlice(bt.blockHeight),
		common.EncodeUint64ToByteSlice(bt.transaction.B.SequenceID),
		common.GetUniqueIDFromUUID(),
	)
}

func (bt BlockTransaction) NewBlockTransactionKeyByBlock(hash string) string {
	return fmt.Sprintf(
		"%s%s%s%s",
		GetBlockTransactionKeyPrefixBlock(hash),
		common.EncodeUint64ToByteSlice(bt.blockHeight),
		common.EncodeUint64ToByteSlice(bt.transaction.B.SequenceID),
		common.GetUniqueIDFromUUID(),
	)
}

func (bt *BlockTransaction) Save(st *storage.LevelDBBackend) (err error) {
	if bt.isSaved {
		return errors.ErrorAlreadySaved
	}

	key := GetBlockTransactionKey(bt.Hash)

	var exists bool
	if exists, err = st.Has(key); exists || err != nil {
		if exists {
			return errors.ErrorBlockAlreadyExists
		}
		return
	}

	if err = st.New(GetBlockTransactionKey(bt.Hash), bt); err != nil {
		return
	}
	if err = st.New(bt.NewBlockTransactionKeySource(), bt.Hash); err != nil {
		return
	}
	if err = st.New(bt.NewBlockTransactionKeyConfirmed(), bt.Hash); err != nil {
		return
	}
	if err = st.New(bt.NewBlockTransactionKeyByAccount(bt.transaction.Source()), bt.Hash); err != nil {
		return
	}
	if err = st.New(bt.NewBlockTransactionKeyByBlock(bt.Block), bt.Hash); err != nil {
		return
	}

	for _, op := range bt.transaction.B.Operations {
		if pop, ok := op.B.(operation.Payable); ok {
			err = st.New(bt.NewBlockTransactionKeyByAccount(pop.TargetAddress()), bt.Hash)
			if err != nil {
				return
			}
		}
	}

	event := "saved"
	event += " " + fmt.Sprintf("source-%s", bt.transaction.Source())
	event += " " + fmt.Sprintf("hash-%s", bt.Hash)
	observer.BlockTransactionObserver.Trigger(event, bt)
	bt.isSaved = true

	if err = SaveTransactionHistory(st, bt.transaction, bt.Message, TransactionHistoryStatusConfirmed); err != nil {
		return
	}

	return nil
}

func (bt BlockTransaction) Serialize() (encoded []byte, err error) {
	encoded, err = common.EncodeJSONValue(bt)
	return
}

func (bt BlockTransaction) String() string {
	encoded, _ := common.EncodeJSONValue(bt)
	return string(encoded)
}

func (bt *BlockTransaction) Transaction() (tx transaction.Transaction) {
	if bt.transaction.IsEmpty() {
		var err error
		if err = bt.LoadTransaction(); err != nil {
			return
		}
	}

	return bt.transaction
}

func GetBlockTransactionKeyPrefixSource(source string) string {
	return fmt.Sprintf("%s%s-", common.BlockTransactionPrefixSource, source)
}

func GetBlockTransactionKeyPrefixConfirmed(confirmed string) string {
	return fmt.Sprintf("%s%s-", common.BlockTransactionPrefixConfirmed, confirmed)
}

func GetBlockTransactionKeyPrefixAccount(accountAddress string) string {
	return fmt.Sprintf("%s%s-", common.BlockTransactionPrefixAccount, accountAddress)
}

func GetBlockTransactionKeyPrefixBlock(hash string) string {
	return fmt.Sprintf("%s%s-", common.BlockTransactionPrefixBlock, hash)
}

func GetBlockTransactionKey(hash string) string {
	return fmt.Sprintf("%s%s", common.BlockTransactionPrefixHash, hash)
}

func GetBlockTransaction(st *storage.LevelDBBackend, hash string) (bt BlockTransaction, err error) {
	if err = st.Get(GetBlockTransactionKey(hash), &bt); err != nil {
		return
	}

	bt.isSaved = true
	return
}

func ExistsBlockTransaction(st *storage.LevelDBBackend, hash string) (bool, error) {
	return st.Has(GetBlockTransactionKey(hash))
}

func LoadBlockTransactionsInsideIterator(
	st *storage.LevelDBBackend,
	iterFunc func() (storage.IterItem, bool),
	closeFunc func(),
) (
	func() (BlockTransaction, bool, []byte),
	func(),
) {

	return (func() (BlockTransaction, bool, []byte) {
			item, hasNext := iterFunc()
			if !hasNext {
				return BlockTransaction{}, false, item.Key
			}

			var hash string
			json.Unmarshal(item.Value, &hash)

			bt, err := GetBlockTransaction(st, hash)
			if err != nil {
				return BlockTransaction{}, false, item.Key
			}

			return bt, hasNext, item.Key
		}), (func() {
			closeFunc()
		})
}

func GetBlockTransactionsBySource(st *storage.LevelDBBackend, source string, options storage.ListOptions) (
	func() (BlockTransaction, bool, []byte),
	func(),
) {
	iterFunc, closeFunc := st.GetIterator(GetBlockTransactionKeyPrefixSource(source), options)

	return LoadBlockTransactionsInsideIterator(st, iterFunc, closeFunc)
}

func GetBlockTransactionsByConfirmed(st *storage.LevelDBBackend, options storage.ListOptions) (
	func() (BlockTransaction, bool, []byte),
	func(),
) {
	iterFunc, closeFunc := st.GetIterator(common.BlockTransactionPrefixConfirmed, options)

	return LoadBlockTransactionsInsideIterator(st, iterFunc, closeFunc)
}

func GetBlockTransactionsByAccount(st *storage.LevelDBBackend, accountAddress string, options storage.ListOptions) (
	func() (BlockTransaction, bool, []byte),
	func(),
) {
	iterFunc, closeFunc := st.GetIterator(GetBlockTransactionKeyPrefixAccount(accountAddress), options)
	return LoadBlockTransactionsInsideIterator(st, iterFunc, closeFunc)
}

func GetBlockTransactionsByBlock(st *storage.LevelDBBackend, hash string, options storage.ListOptions) (
	func() (BlockTransaction, bool, []byte),
	func(),
) {
	iterFunc, closeFunc := st.GetIterator(GetBlockTransactionKeyPrefixBlock(hash), options)
	return LoadBlockTransactionsInsideIterator(st, iterFunc, closeFunc)
}

var GetBlockTransactions = GetBlockTransactionsByConfirmed
