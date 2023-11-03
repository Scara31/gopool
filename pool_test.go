package gopool

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestGoPool(t *testing.T) {
	testCases := []struct {
		data   []string
		result string
	}{
		{
			[]string{"1", "12", "123", "1234", "12345"},
			"1234",
		},
		{
			[]string{"1", "12", "123", "12345"},
			"finished",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("want %s for %v", tc.result, tc.data), func(t *testing.T) {
			gp := NewGoPool(len(tc.data), func(gp *GoPool, v any) {
				s := v.(string)
				if len(s) == 4 {
					gp.OutChan <- s
				}
			})

			for _, v := range tc.data {
				gp.InChan <- v
			}

			doneChan := gp.CollectResults()

			var result string
			select {
			case <-doneChan:
				result = "finished"
			case v := <-gp.OutChan:
				result = v.(string)
			}

			if result != tc.result {
				t.Errorf("results differ, got %s, want %s", result, tc.result)
			}
		})
	}
}

func sleepyWorkerFunction(gp *GoPool, v any) {
	time.Sleep(sleepTime)
}

const sleepTime = time.Second
const sleepyWorkersNumber = 3

func TestShrink(t *testing.T) {
	// it takes 1 second to execute the sleepyWorkerFunction
	gp := NewGoPool(sleepyWorkersNumber, sleepyWorkerFunction)
	// leave only 1 running worker
	gp.Shrink(sleepyWorkersNumber - 1)

	t1 := time.Now()

	for i := 0; i < sleepyWorkersNumber; i++ {
		gp.InChan <- struct{}{}
	}

	doneChan := gp.CollectResults()
	<-doneChan

	t2 := time.Now()

	// check that it took duration = sleepyWorkersNumber * second
	d := int(t2.Sub(t1).Seconds())
	if d != sleepyWorkersNumber {
		t.Errorf("shrink failed, d=%d", d)
	}
}

func TestGrow(t *testing.T) {
	// it takes 1 second to execute the sleepyWorkerFunction
	gp := NewGoPool(1, sleepyWorkerFunction)
	// grow so much workers, that tasksNumber == sleepyWorkersNumber
	gp.Grow(sleepyWorkersNumber - 1)

	t1 := time.Now()

	for i := 0; i < sleepyWorkersNumber; i++ {
		gp.InChan <- struct{}{}
	}

	doneChan := gp.CollectResults()
	<-doneChan

	t2 := time.Now()

	d := int(t2.Sub(t1).Seconds())
	// check that it took only 1 second
	if d != 1 {
		t.Errorf("grow failed, d=%d", d)
	}
}

const urlAddr = "http://localhost:8080"
const requestsNumber = 1000

func setupTestServer() *http.Server {
	srv := &http.Server{
		Addr: ":8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
		}),
	}

	go srv.ListenAndServe()
	return srv
}

func BenchmarkSyncHTTP(b *testing.B) {
	srv := setupTestServer()
	defer srv.Shutdown(context.Background())

	b.ResetTimer()
	for i := 0; i < requestsNumber; i++ {
		if _, err := http.Get(urlAddr); err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	srv.Shutdown(context.Background())
}

func BenchmarkAsyncHTTP(b *testing.B) {
	srv := setupTestServer()

	gp := NewGoPool(requestsNumber, func(gp *GoPool, v any) {
		url := v.(string)
		if _, err := http.Get(url); err != nil {
			b.Fatal(err)
		}
	})

	b.ResetTimer()
	for i := 0; i < 100; i++ {
		gp.InChan <- urlAddr
	}

	doneChan := gp.CollectResults()

	<-doneChan

	b.StopTimer()
	srv.Shutdown(context.Background())
}
