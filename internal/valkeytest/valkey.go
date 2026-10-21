// Package valkeytest starts a Valkey container for tests of the shared cache. Docker is required: a test that cannot
// start one fails rather than skips.
package valkeytest

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Image is the Valkey the tests run against.
const Image = "valkey/valkey:8-alpine"

// Container is a running Valkey that a test can stop and start again.
type Container struct {
	container testcontainers.Container
	URL       string
}

// Start runs Valkey for the test and stops it when the test ends.
func Start(t *testing.T) *Container {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Image:        Image,
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp"),
		Started:      true,
	})
	require.NoError(t, err, "Docker is needed to run Valkey")
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("valkey was not stopped: %v", err)
		}
	})
	host, err := container.Host(ctx)
	require.NoError(t, err)
	if host == "localhost" {
		// Every new connection would resolve localhost and try [::1] first, which Colima does not forward; a burst of
		// them then outlasts the cache's 200 ms dial timeout.
		host = "127.0.0.1"
	}
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	return &Container{container: container, URL: "redis://" + net.JoinHostPort(host, port.Port())}
}
