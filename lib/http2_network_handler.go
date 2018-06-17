package sebak

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"boscoin.io/sebak/lib/network"
	"boscoin.io/sebak/lib/storage"
)

func GetAccountHandler(ctx context.Context, t *sebaknetwork.HTTP2Network) sebaknetwork.HandlerFunc {
	storage, ok := ctx.Value("storage").(*sebakstorage.LevelDBBackend)
	if !ok {
		panic(errors.New("storage is missing in context"))
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		k := strings.SplitN(r.URL.Path, "/", 3)
		if len(k) != 3 {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		address := k[2]
		if found, err := ExistBlockAccount(storage, address); err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		} else if !found {
			http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		var err error
		switch r.Header.Get("Accept") {
		case "text/event-stream":
			cn, ok := w.(http.CloseNotifier)
			if !ok {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			closeChan := make(chan bool)
			messageChan := make(chan []byte)

			BlockAccountObserver.On(fmt.Sprintf("saved-%s", address), func(args ...interface{}) {
				ba := args[0].(*BlockAccount)
				s, err := ba.Serialize()
				if err != nil {
					closeChan <- true
					return
				}
				messageChan <- s
			})

			w.Header().Set("Content-Type", "application/json")

			// current BlockAccount data will be sent
			var ba *BlockAccount
			if ba, err = GetBlockAccount(storage, address); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s, err := ba.Serialize()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			fmt.Fprintf(w, "%s\n", s)
			flusher.Flush()

			// this code will increase balance by 2 seconds
			go func() {
				var n int
				for {
					newBalance, _ := ba.GetBalance().Add(Amount(n))
					ba.Balance = newBalance.String()

					ba.Save(storage)
					n++
					time.Sleep(2 * time.Second)
				}
			}()

			for {
				select {
				case <-closeChan:
					return
				case <-cn.CloseNotify():
					return
				case message := <-messageChan:
					fmt.Fprintf(w, "%s\n", message)
					flusher.Flush()
				}
			}
		default:
			var ba *BlockAccount
			if ba, err = GetBlockAccount(storage, address); err != nil {
				http.Error(w, "Error reading request body", http.StatusInternalServerError)
				return
			}

			var s []byte
			if s, err = ba.Serialize(); err != nil {
				http.Error(w, "Error reading request body", http.StatusInternalServerError)
				return
			}

			if _, err = w.Write(s); err != nil {
				http.Error(w, "Error reading request body", http.StatusInternalServerError)
				return
			}
		}
	}
}
