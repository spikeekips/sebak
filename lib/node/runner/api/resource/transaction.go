package resource

import (
	"strings"

	"github.com/nvellon/hal"

	"boscoin.io/sebak/lib/block"
)

type Transaction struct {
	bt *block.BlockTransaction
}

func NewTransaction(bt *block.BlockTransaction) *Transaction {
	_ = bt.LoadTransaction()

	t := &Transaction{
		bt: bt,
	}
	return t
}

func (t Transaction) GetMap() hal.Entry {
	return hal.Entry{
		"hash":            t.bt.Hash,
		"source":          t.bt.Transaction().Source(),
		"fee":             t.bt.Transaction().B.Fee.String(),
		"sequence_id":     t.bt.Transaction().B.SequenceID,
		"created":         t.bt.Transaction().H.Created,
		"operation_count": len(t.bt.Transaction().B.Operations),
	}
}

func (t Transaction) Resource() *hal.Resource {
	r := hal.NewResource(t, t.LinkSelf())
	r.AddLink("account", hal.NewLink(strings.Replace(URLAccounts, "{id}", t.bt.Transaction().Source(), -1)))
	r.AddLink("operations", hal.NewLink(strings.Replace(URLTransactionOperations, "{id}", t.bt.Hash, -1)+"{?cursor,limit,order}", hal.LinkAttr{"templated": true}))
	return r
}

func (t Transaction) LinkSelf() string {
	return strings.Replace(URLTransactions, "{id}", t.bt.Hash, -1)
}
