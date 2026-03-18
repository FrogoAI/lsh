// Package testdata provides shared helpers for integration tests.
package testdata

import (
	"fmt"
	"time"

	as "github.com/aerospike/aerospike-client-go/v8"
)

const (
	// Host is the default Aerospike test host.
	Host = "127.0.0.1"
	// Port is the default Aerospike test port.
	Port = 3000
	// Namespace is the default Aerospike test namespace.
	Namespace = "test"
)

// WaitForAerospike blocks until Aerospike is fully ready to accept operations.
// Probes with write, read, delete, and batch — no truncate (which causes FAIL_FORBIDDEN).
func WaitForAerospike(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := probeAerospike(); err != nil {
			fmt.Printf("Aerospike not ready: %v\n", err) //nolint:forbidigo
			time.Sleep(2 * time.Second)                   //nolint:mnd

			continue
		}

		return nil
	}

	return fmt.Errorf("aerospike not ready after %s", timeout)
}

func probeAerospike() error {
	client, err := as.NewClient(Host, Port)
	if err != nil {
		return err
	}

	defer client.Close()

	key, err := as.NewKey(Namespace, "readiness", "probe")
	if err != nil {
		return err
	}

	if err := client.Put(nil, key, as.BinMap{"ok": 1}); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	rec, err := client.Get(nil, key)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	if rec == nil {
		return fmt.Errorf("read: record not found after write")
	}

	if _, err := client.Delete(nil, key); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	keys := make([]*as.Key, 2) //nolint:mnd
	for i := range keys {
		keys[i], _ = as.NewKey(Namespace, "readiness", fmt.Sprintf("b%d", i))
	}

	if _, err := client.BatchGet(nil, keys, "ok"); err != nil {
		return fmt.Errorf("batch: %w", err)
	}

	return nil
}
