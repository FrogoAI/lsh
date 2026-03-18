//go:build ignore

// wait_aerospike.go waits until Aerospike is ready to accept operations.
// It connects, verifies the "test" namespace exists, and performs a test write+delete.
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
	timeout := 60 * time.Second

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		client, err := as.NewClient(host, port)
		if err != nil {
			fmt.Printf("Connecting... %v\n", err)
			time.Sleep(2 * time.Second)

			continue
		}

		// Verify namespace is available by doing a test write
		key, err := as.NewKey(namespace, "readiness", "probe")
		if err != nil {
			client.Close()
			fmt.Printf("Key error: %v\n", err)
			time.Sleep(2 * time.Second)

			continue
		}

		err = client.Put(nil, key, as.BinMap{"ok": 1})
		if err != nil {
			client.Close()
			fmt.Printf("Write probe... %v\n", err)
			time.Sleep(2 * time.Second)

			continue
		}

		// Clean up probe record
		_, _ = client.Delete(nil, key)
		client.Close()

		fmt.Println("Aerospike ready")
		os.Exit(0)
	}

	fmt.Println("Aerospike not ready after timeout")
	os.Exit(1)
}
