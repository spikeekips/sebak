package sebak

import (
	"sync"
	"testing"

	"boscoin.io/sebak/lib/storage"
)

func TestSaveNewBlockAccount(t *testing.T) {
	st, _ := sebakstorage.NewTestMemoryLevelDBBackend()

	b := testMakeBlockAccount()
	err := b.Save(st)
	if err != nil {
		t.Errorf("failed to save BlockAccount, %v", err)
		return
	}

	exists, err := ExistBlockAccount(st, b.Address)
	if err != nil {
		t.Errorf("failed to get BlockAccount, %v", err)
		return
	}

	if !exists {
		t.Errorf("failed to get BlockAccount, does not exists")
		return
	}
}

func TestSaveExistingBlockAccount(t *testing.T) {
	st, _ := sebakstorage.NewTestMemoryLevelDBBackend()

	b := testMakeBlockAccount()
	b.Save(st)

	if err := b.Deposit(Amount(100), "fake-checkpoint"); err != nil {
		panic(err)
	}
	if err := b.Save(st); err != nil {
		panic(err)
	}

	fetched, _ := GetBlockAccount(st, b.Address)
	if b.Balance != fetched.Balance {
		t.Error("failed to update `BlockAccount.Balance`")
		return
	}
}

func TestSortMultipleBlockAccount(t *testing.T) {
	st, _ := sebakstorage.NewTestMemoryLevelDBBackend()

	var createdOrder []string
	for i := 0; i < 50; i++ {
		b := testMakeBlockAccount()
		b.Save(st)

		createdOrder = append(createdOrder, b.Address)
	}

	var saved []string
	iterFunc, closeFunc := GetBlockAccountAddressesByCreated(st, false)
	for {
		address, hasNext := iterFunc()
		if !hasNext {
			break
		}

		saved = append(saved, address)
	}
	closeFunc()

	for i, a := range createdOrder {
		if a != saved[i] {
			t.Error("failed to save `BlockAccount` by creation order")
			break
		}
	}
}

func TestGetSortedBlockAccounts(t *testing.T) {
	st, _ := sebakstorage.NewTestMemoryLevelDBBackend()

	var createdOrder []string
	for i := 0; i < 50; i++ {
		b := testMakeBlockAccount()
		b.Save(st)

		createdOrder = append(createdOrder, b.Address)
	}

	var saved []string
	iterFunc, closeFunc := GetBlockAccountsByCreated(st, false)
	for {
		ba, hasNext := iterFunc()
		if !hasNext {
			break
		}

		saved = append(saved, ba.Address)
	}
	closeFunc()

	for i, a := range createdOrder {
		if a != saved[i] {
			t.Error("failed to save `BlockAccount` by creation order")
			break
		}
	}
}

func TestBlockAccountObserver(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	var triggered *BlockAccount
	BlockAccountObserver.On("saved", func(args ...interface{}) {
		triggered = args[0].(*BlockAccount)
		wg.Done()
	})

	st, _ := sebakstorage.NewTestMemoryLevelDBBackend()

	b := testMakeBlockAccount()
	b.Save(st)

	wg.Wait()

	if b.Address != triggered.Address {
		t.Error("Address is not match")
		return
	}
	if b.Balance != triggered.Balance {
		t.Error("Balance is not match")
		return
	}
	if b.Checkpoint != triggered.Checkpoint {
		t.Error("Checkpoint is not match")
		return
	}
}
