//go:build linux

package deploymentsecret

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestUnixPeerUIDReadsKernelCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deployment-secret.sock")
	listener, err := net.ListenUnix(unixSocketNetwork, &net.UnixAddr{Name: path, Net: unixSocketNetwork})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	accepted := make(chan *net.UnixConn, 1)
	go func() {
		conn, acceptErr := listener.AcceptUnix()
		if acceptErr == nil {
			accepted <- conn
		}
	}()
	client, err := net.DialUnix(unixSocketNetwork, nil, &net.UnixAddr{Name: path, Net: unixSocketNetwork})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	server := <-accepted
	t.Cleanup(func() { _ = server.Close() })

	uid, err := UnixPeerUID(server)
	if err != nil {
		t.Fatal(err)
	}
	if uid != uint32(os.Getuid()) {
		t.Fatalf("UnixPeerUID() = %d, want %d", uid, os.Getuid())
	}
}
