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
			gp := NewGoPool[string, string](
				len(tc.data),
				context.Background(),
				make(chan string),
				make(chan string),
				func(gp *GoPool[string, string], v string) {
					if len(v) == 4 {
						gp.OutChan <- v
					}
				})

			for _, v := range tc.data {
				gp.InChan <- v
			}

			gp.CloseInChan()
			// Set default value
			result := "finished"

			// If any value was received, it will overwrite the initial one
			for v := range gp.OutChan {
				result = v
			}

			if result != tc.result {
				t.Errorf("results differ, got %s, want %s", result, tc.result)
			}
		})
	}
}

func sleepyWorkerFunction[T1 struct{}, T2 struct{}](gp *GoPool[T1, T2], v T1) {
	time.Sleep(sleepTime)
}

const sleepTime = time.Second
const sleepyWorkersNumber = 3

func TestShrink(t *testing.T) {
	// it takes 1 second to execute the sleepyWorkerFunction
	gp := NewGoPool[struct{}, struct{}](
		sleepyWorkersNumber,
		context.Background(),
		make(chan struct{}),
		make(chan struct{}),
		sleepyWorkerFunction[struct{}, struct{}],
	)
	// leave only 1 running worker
	gp.Shrink(sleepyWorkersNumber - 1)

	t1 := time.Now()

	for i := 0; i < sleepyWorkersNumber; i++ {
		gp.InChan <- struct{}{}
	}

	gp.CloseInChan()
	<-gp.OutChan

	t2 := time.Now()

	// check that it took duration = sleepyWorkersNumber * second
	d := int(t2.Sub(t1).Seconds())
	if d != sleepyWorkersNumber {
		t.Errorf("shrink failed, d=%d", d)
	}
}

func TestGrow(t *testing.T) {
	// it takes 1 second to execute the sleepyWorkerFunction
	gp := NewGoPool[struct{}, struct{}](
		1,
		context.Background(),
		make(chan struct{}),
		make(chan struct{}),
		sleepyWorkerFunction[struct{}, struct{}],
	)
	// grow so much workers, that tasksNumber == sleepyWorkersNumber
	gp.Grow(sleepyWorkersNumber - 1)

	t1 := time.Now()

	for i := 0; i < sleepyWorkersNumber; i++ {
		gp.InChan <- struct{}{}
	}

	gp.CloseInChan()
	<-gp.OutChan

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

	gp := NewGoPool[string, struct{}](
		requestsNumber,
		context.Background(),
		make(chan string),
		make(chan struct{}),
		func(gp *GoPool[string, struct{}], url string) {
			if _, err := http.Get(url); err != nil {
				b.Fatal(err)
			}
		})

	b.ResetTimer()
	for i := 0; i < 100; i++ {
		gp.InChan <- urlAddr
	}

	gp.CloseInChan()
	<-gp.OutChan

	b.StopTimer()
	srv.Shutdown(context.Background())
}
