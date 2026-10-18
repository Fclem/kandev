package plugins

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMessageListAuthorTypesFilter(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{APIRead: []string{"messages"}})
	filter := pluginsdk.MessageFilter{AuthorTypes: []string{"user"}}
	if _, _, err := d.host.Messages().List(context.Background(), filter, pluginsdk.Page{}); err != nil {
		t.Fatalf("Messages().List: %v", err)
	}
	if got := d.messages.lastFilter.AuthorTypes; !reflect.DeepEqual(got, []string{"user"}) {
		t.Fatalf("forwarded author types = %v, want [user]", got)
	}

	tooMany := pluginsdk.MessageFilter{Types: make([]string, maxMessageFilterValues), AuthorTypes: []string{"user"}}
	_, _, err := d.host.Messages().List(context.Background(), tooMany, pluginsdk.Page{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("filter error = %v, want InvalidArgument for combined 401 values", err)
	}
	if d.messages.calls != 1 {
		t.Fatalf("data source calls = %d, want only the accepted request", d.messages.calls)
	}
}
