//go:build ignore

// wait_aerospike waits until Aerospike is fully ready to accept all operations.
// It connects, performs a write, truncate, and batch operation to verify full readiness.
package main

import (
	"fmt"
	"os"
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"
)

func main() {
	host := "127.0.0.1"
	port := 3000
	namespace := "test"
	timeout := 90 * time.Second

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if tryReady(host, port, namespace) {
			fmt.Println("Aerospike ready")
			os.Exit(0)
		}

		time.Sleep(2 * time.Second)
	}

	fmt.Println("Aerospike not ready after timeout")
	os.Exit(1)
}

func tryReady(host string, port int, namespace string) bool {
	client, err := as.NewClient(host, port)
	if err != nil {
		fmt.Printf("Connecting... %v\n", err)

		return false
	}

	defer client.Close()

	// 1. Verify simple write
	key, err := as.NewKey(namespace, "readiness", "probe")
	if err != nil {
		fmt.Printf("Key error: %v\n", err)

		return false
	}

	err = client.Put(nil, key, as.BinMap{"ok": 1})
	if err != nil {
		fmt.Printf("Write probe... %v\n", err)

		return false
	}

	// 2. Verify truncate works (tests use truncate before each test)
	err = client.Truncate(nil, namespace, "readiness", nil)
	if err != nil {
		fmt.Printf("Truncate probe... %v\n", err)

		return false
	}

	// 3. Verify batch operations work
	keys := make([]*as.Key, 3) //nolint:mnd

	for i := range keys {
		keys[i], _ = as.NewKey(namespace, "readiness", fmt.Sprintf("batch_%d", i))
	}

	_, err = client.BatchGet(nil, keys, "ok")
	if err != nil {
		fmt.Printf("Batch probe... %v\n", err)

		return false
	}

	return true
}
