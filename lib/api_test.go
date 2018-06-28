package sebak

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"boscoin.io/sebak/lib/common"
	"boscoin.io/sebak/lib/storage"
)

func TestStreaming(t *testing.T) {
	// Setting Server
	storageConfig, err := sebakstorage.NewConfigFromString("memory://")
	if err != nil {
		t.Errorf("failed to initialize file db: %v", err)

	}
	storage, err := sebakstorage.NewStorage(storageConfig)
	if err != nil {
		t.Errorf("failed to initialize file db: %v", err)
	}
	defer storage.Close()

	host := fmt.Sprintf("localhost:%d", sebakcommon.GetFreePort())

	router := mux.NewRouter()
	router.HandleFunc("/account/{address}", GetAccountHandler(storage)).Methods("GET")
	server := &http.Server{Addr: host, Handler: router}
	go server.ListenAndServe()
	defer func() {
		timer := time.NewTimer(500 * time.Millisecond)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			server.Shutdown(ctx)
		}()
		<-timer.C
		cancel()
	}()

	// check connection availability
	for {
		if _, err := net.DialTimeout("tcp", host, 100*time.Millisecond); err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		break
	}

	// Make Dummy BlockAccount
	ba := testMakeBlockAccount()
	ba.Save(storage)
	prev := ba.GetBalance()

	var wg sync.WaitGroup
	wg.Add(1)

	// Do stream Request to the Server
	go func() {
		defer wg.Done()

		req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/account/%s", host, ba.Address), nil)
		if err != nil {
			t.Errorf("failed to make request: %v", err)
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("Server not yet initialized: %v", err)
			return
		}

		if err != nil {
			for {
				resp, err = http.DefaultClient.Do(req)
				if err == nil {
					break
				}
				time.Sleep(time.Second)
			}
		}
		reader := bufio.NewReader(resp.Body)
		var n Amount
		for n = 1; n < 10; n++ {
			line, _ := reader.ReadBytes('\n')
			var cba = &BlockAccount{}
			json.Unmarshal(line, cba)

			if ba.Address != cba.Address {
				t.Errorf("Address: Expected=%s Actual=%s", ba.Address, cba.Address)
			}
			if cba.GetBalance()-prev != n {
				t.Errorf("Balance: Expected=%d Actual=%d", prev+n, cba.GetBalance())
			}
			prev = cba.GetBalance()
		}
		resp.Body.Close()
	}()

	// Makes Some Events
	for n := 1; n < 20; n++ {
		newBalance, _ := ba.GetBalance().Add(Amount(n))
		ba.Balance = newBalance.String()

		ba.Save(storage)
		time.Sleep(time.Millisecond * 100)
	}

	wg.Wait()
}
