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

// NewReadyClient creates an Aerospike client and blocks until the server
// is fully ready to accept operations. Returns the connected client.
// The caller is responsible for closing the client.
func NewReadyClient(timeout time.Duration) (*as.Client, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		client, err := as.NewClient(Host, Port)
		if err != nil {
			fmt.Printf("Aerospike connecting... %v\n", err) //nolint:forbidigo
			time.Sleep(2 * time.Second)                      //nolint:mnd

			continue
		}

		if err := probe(client); err != nil {
			client.Close()
			fmt.Printf("Aerospike not ready: %v\n", err) //nolint:forbidigo
			time.Sleep(2 * time.Second)                   //nolint:mnd

			continue
		}

		return client, nil
	}

	return nil, fmt.Errorf("aerospike not ready after %s", timeout)
}

func probe(client *as.Client) error {
	key, err := as.NewKey(Namespace, "readiness", "probe")
	if err != nil {
		return err
	}

	if err := client.Put(nil, key, as.BinMap{"ok": 1}); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	if _, err := client.Delete(nil, key); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	return nil
}
