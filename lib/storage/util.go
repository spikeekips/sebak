package storage

import (
	"encoding/json"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"

	"boscoin.io/sebak/lib/common"
)

func Serialize(v interface{}) (encoded []byte, err error) {
	serializable, ok := v.(common.Serializable)
	if ok {
		encoded, err = serializable.Serialize()
	} else {
		encoded, err = common.EncodeJSONValue(v)
	}
	if err != nil {
		err = setLevelDBCoreError(err)
		return
	}

	return
}

func Unserialize(b []byte, i interface{}) (err error) {
	if err = json.Unmarshal(b, i); err != nil {
		err = setLevelDBCoreError(err)
		return
	}

	return
}

type BatchElement struct {
	Key     string
	Content interface{}
}

type Batch struct {
	sync.RWMutex

	instances map[string]interface{}
	contents  []BatchElement
}

func NewBatch() *Batch {
	return &Batch{
		instances: map[string]interface{}{},
	}
}

func (b *Batch) Get(key string) (instance interface{}, found bool) {
	b.RLock()
	defer b.RUnlock()

	instance, found = b.instances[key]
	return
}

func (b *Batch) Set(key string, instance interface{}) {
	b.Lock()
	defer b.Unlock()

	b.instances[key] = instance
}

func (b *Batch) Put(key string, content interface{}) {
	b.RLock()
	defer b.RUnlock()

	b.contents = append(b.contents, BatchElement{Key: key, Content: content})
}

func (b *Batch) Range() <-chan BatchElement {
	ch := make(chan BatchElement)
	go func() {
		b.RLock()
		defer b.RUnlock()

		for _, element := range b.contents {
			ch <- element
		}
		close(ch)
	}()

	return ch
}

func (b *Batch) RangeInstance() <-chan interface{} {
	ch := make(chan interface{})
	go func() {
		b.RLock()
		defer b.RUnlock()

		for _, instance := range b.instances {
			ch <- instance
		}
		close(ch)
	}()

	return ch
}

func (b *Batch) LeveldbBatch() (batch *leveldb.Batch, err error) {
	batch = &leveldb.Batch{}

	var encoded []byte
	for element := range b.Range() {
		encoded, err = Serialize(element.Content)
		if err != nil {
			return
		}
		batch.Put([]byte(element.Key), encoded)
	}

	return
}

func (b *Batch) Write(st *LevelDBBackend) (err error) {
	var batches *leveldb.Batch
	if batches, err = b.LeveldbBatch(); err != nil {
		return
	}

	err = st.BatchWrite(batches)

	return
}
