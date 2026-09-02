package websocket

import (
	"context"
	"testing"
	"time"
)

func TestHub_ClientLifecycleCallsReturnAfterShutdown(t *testing.T) {
	h := newTestHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		h.Run(ctx)
	}()
	cancel()
	<-hubDone

	for name, lifecycleCall := range map[string]func(*Client){
		"register":   h.Register,
		"unregister": h.Unregister,
	} {
		t.Run(name, func(t *testing.T) {
			client := newTestClient(name)
			returned := make(chan struct{})
			go func() {
				lifecycleCall(client)
				close(returned)
			}()

			select {
			case <-returned:
			case <-time.After(100 * time.Millisecond):
				t.Fatal("client lifecycle call blocked after hub shutdown")
			}
			if !client.closed {
				t.Fatal("client send channel remained open after hub shutdown")
			}
		})
	}
}

func TestHubClientDisconnectNotifiesListenerOnce(t *testing.T) {
	h := newTestHub(t)
	client := newTestClient("connection-a")
	h.clients[client] = true

	var disconnected []string
	h.SetClientDisconnectListener(func(connectionID string) {
		disconnected = append(disconnected, connectionID)
	})

	h.removeClient(client)
	h.removeClient(client)

	if len(disconnected) != 1 || disconnected[0] != client.ID {
		t.Fatalf("disconnect notifications = %#v, want one notification for %q", disconnected, client.ID)
	}
}
