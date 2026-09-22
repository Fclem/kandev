package deploymentsecret

import "testing"

func TestAuthorizeRootPeerRejectsUnprivilegedUID(t *testing.T) {
	for _, uid := range []uint32{1, 1000, 65534} {
		if err := AuthorizeRootPeer(uid); err == nil {
			t.Fatalf("AuthorizeRootPeer(%d) succeeded, want rejection", uid)
		}
	}
}

func TestAuthorizeRootPeerAcceptsRootUID(t *testing.T) {
	if err := AuthorizeRootPeer(0); err != nil {
		t.Fatalf("AuthorizeRootPeer(0) error = %v", err)
	}
}
