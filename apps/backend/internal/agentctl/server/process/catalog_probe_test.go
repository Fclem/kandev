//go:build !windows

package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/sessioncatalog"
	"github.com/kandev/kandev/pkg/agent"
)

const catalogProbeTestInstance = "instance-under-test"

type catalogProbeFixture struct {
	dir     string
	work    string
	mgr     *Manager
	request sessioncatalog.TaskProbeRequest
	livePID int
}

func catalogPage(next string, ids ...string) json.RawMessage {
	data := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]any{"id": id, "model": id, "displayName": strings.ToUpper(id), "description": id + " description"})
	}
	page := map[string]any{"data": data, "nextCursor": nil}
	if next != "" {
		page["nextCursor"] = next
	}
	payload, _ := json.Marshal(page)
	return payload
}

func writeFakeCodexScript(t *testing.T, dir string, script fakeCodexScript) {
	t.Helper()
	payload, err := json.Marshal(script)
	if err != nil {
		t.Fatalf("encode script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.json"), payload, 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
}

// fixtureCodexQualifier admits the fixture executable through the production
// native acquisition. The production table additionally requires the exact
// managed package and version, covered separately.
func fixtureCodexQualifier(launch *catalogLaunch) (catalogProbeCapability, bool) {
	qualification, _ := codexQualificationFor("@openai/codex@0.169.0")
	return codexModelListCapability(qualification), launch.protocol == string(agent.ProtocolCodexAppServer) && !launch.wrapped
}

func startCatalogProbeFixture(t *testing.T, live fakeCodexScript) *catalogProbeFixture {
	t.Helper()
	dir := t.TempDir()
	writeFakeCodexScript(t, dir, live)
	work := t.TempDir()
	env := append(os.Environ(), fakeCodexAppServerEnv+"="+dir, "CATALOG_EMPTY_VALUE=", "CATALOG_SECRET=task-secret")
	mgr := newRestartManager(t, &config.InstanceConfig{
		WorkDir: work, AgentArgs: fixtureArgs(), AgentEnv: env,
		Protocol: agent.ProtocolCodexAppServer, AgentType: "codex-app-server",
	})
	mgr.catalogProbeQualifier = fixtureCodexQualifier
	startRestartManager(t, mgr)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	live_ := mgr.GetAdapter()
	if err := live_.Initialize(ctx); err != nil {
		t.Fatalf("initialize live adapter: %v", err)
	}
	sessionID, err := live_.NewSession(ctx, nil)
	if err != nil {
		t.Fatalf("start live thread: %v", err)
	}
	fixture := &catalogProbeFixture{dir: dir, work: work, mgr: mgr, livePID: mgr.cmd.Process.Pid}
	fixture.request = sessioncatalog.TaskProbeRequest{
		RequestID: "probe-1", ManagerID: mgr.ProcessManagerID(), ProcessGeneration: mgr.ProcessGeneration(),
		SessionID: sessionID, SessionSource: SessionCatalogSource(live_),
	}
	return fixture
}

func (f *catalogProbeFixture) discover(ctx context.Context) (sessioncatalog.TaskProbeReply, error) {
	return f.mgr.DiscoverCatalogCandidates(ctx, catalogProbeTestInstance, f.request)
}

func (f *catalogProbeFixture) helperPID(t *testing.T) string {
	t.Helper()
	frames, err := fakeCodexFrames(f.dir)
	if err != nil {
		t.Fatalf("read frames: %v", err)
	}
	for pid := range frames {
		if pid != strconv.Itoa(f.livePID) {
			return pid
		}
	}
	t.Fatal("no helper process was observed")
	return ""
}

func readFixtureFile(t *testing.T, dir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func processExited(pid int) bool {
	err := syscall.Kill(pid, 0)
	return errors.Is(err, syscall.ESRCH)
}

func waitForCatalogFile(t *testing.T, pattern string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if matches, _ := filepath.Glob(pattern); len(matches) == 1 {
			return matches[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", pattern)
	return ""
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
// @covers AC-AGENTS-SESSION-CATALOG-001.8
func TestSessionCatalogTaskDiscoveryFreshHelperUsesSealedLaunch(t *testing.T) {
	fixture := startCatalogProbeFixture(t, fakeCodexScript{Pages: map[string]json.RawMessage{"": catalogPage("", "live-a")}})
	writeFakeCodexScript(t, fixture.dir, fakeCodexScript{Pages: map[string]json.RawMessage{
		"": catalogPage("cursor-2", "fresh-b1"), "cursor-2": catalogPage("", "fresh-b2"),
	}})

	reply, err := fixture.discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if reply.Result.Outcome != sessioncatalog.OutcomeComplete || !reply.Result.Models.Present || !reply.Result.Models.Supported {
		t.Fatalf("result = %+v, want complete supported model surface", reply.Result)
	}
	ids := make([]string, 0, len(reply.Result.Models.Choices))
	for _, choice := range reply.Result.Models.Choices {
		ids = append(ids, choice.ID)
	}
	if !slices.Equal(ids, []string{"fresh-b1", "fresh-b2"}) {
		t.Fatalf("models = %v, want the fresh paginated catalog", ids)
	}
	if reply.Result.Modes.Supported || reply.Result.Modes.Present || reply.Result.Modes.Choices != nil {
		t.Fatalf("native modes = %+v, want explicitly unsupported", reply.Result.Modes)
	}
	want := sessioncatalog.TaskProbeOwner{
		InstanceID: catalogProbeTestInstance, ManagerID: fixture.request.ManagerID,
		ProcessGeneration: fixture.request.ProcessGeneration, SessionID: fixture.request.SessionID,
		SessionSource: fixture.request.SessionSource,
	}
	if reply.Owner != want || reply.RequestID != "probe-1" || reply.Capability == "" {
		t.Fatalf("reply owner = %+v request=%q capability=%q", reply.Owner, reply.RequestID, reply.Capability)
	}
	if err := sessioncatalog.ValidateTaskProbeReply(fixture.request, catalogProbeTestInstance, reply); err != nil {
		t.Fatalf("reply does not validate: %v", err)
	}

	frames, err := fakeCodexFrames(fixture.dir)
	if err != nil {
		t.Fatalf("frames: %v", err)
	}
	helper := fixture.helperPID(t)
	if got := frames[helper]; !slices.Equal(got, []string{"initialize", "initialized", "model/list", "model/list"}) {
		t.Fatalf("helper frames = %v, want only initialize and paginated model/list", got)
	}
	if got := frames[strconv.Itoa(fixture.livePID)]; !slices.Equal(got, []string{"initialize", "initialized", "model/list", "thread/start"}) {
		t.Fatalf("live frames = %v, want no frame from the probe", got)
	}
	modelState, ok := fixture.mgr.GetAdapter().(interface {
		GetSessionModelState() *streams.SessionModelState
	})
	if !ok {
		t.Fatal("live adapter does not expose its advertisement")
	}
	if state := modelState.GetSessionModelState(); len(state.Models) != 1 || state.Models[0].ModelID != "live-a" {
		t.Fatalf("live advertisement = %+v, want unchanged live-a", state.Models)
	}
	if fixture.mgr.Status() != StatusRunning || processExited(fixture.livePID) {
		t.Fatal("probe must not stop the live agent")
	}
	livePID := strconv.Itoa(fixture.livePID)
	for _, name := range []string{"env-%s.json", "args-%s.json", "cwd-%s.txt"} {
		live := readFixtureFile(t, fixture.dir, fmt.Sprintf(name, livePID))
		fresh := readFixtureFile(t, fixture.dir, fmt.Sprintf(name, helper))
		if string(live) != string(fresh) {
			t.Fatalf("%s differs between live and helper:\nlive=%s\nhelper=%s", name, live, fresh)
		}
	}
	var helperEnv []string
	if err := json.Unmarshal(readFixtureFile(t, fixture.dir, "env-"+helper+".json"), &helperEnv); err != nil {
		t.Fatalf("decode env: %v", err)
	}
	if !slices.Contains(helperEnv, "CATALOG_EMPTY_VALUE=") || !slices.Contains(helperEnv, "CATALOG_SECRET=task-secret") {
		t.Fatal("helper environment lost an explicit empty value or a task credential")
	}
	pid, _ := strconv.Atoi(helper)
	if !processExited(pid) {
		t.Fatal("helper process must be reaped after the reply")
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryResultBounds(t *testing.T) {
	many := make([]string, 0, sessioncatalog.MaxTaskProbeChoices+1)
	for index := range sessioncatalog.MaxTaskProbeChoices + 1 {
		many = append(many, fmt.Sprintf("model-%d", index))
	}
	manyPages := map[string]json.RawMessage{}
	for index := range sessioncatalog.MaxTaskProbePages + 1 {
		cursor := ""
		if index > 0 {
			cursor = strconv.Itoa(index)
		}
		manyPages[cursor] = catalogPage(strconv.Itoa(index+1), "page-model-"+strconv.Itoa(index))
	}
	cases := []struct {
		name      string
		script    fakeCodexScript
		outcome   string
		wantEmpty bool
	}{
		{name: "explicit empty is complete", script: fakeCodexScript{Pages: map[string]json.RawMessage{"": json.RawMessage(`{"data":[],"nextCursor":null}`)}}, outcome: sessioncatalog.OutcomeComplete, wantEmpty: true},
		{name: "null data fails", script: fakeCodexScript{Pages: map[string]json.RawMessage{"": json.RawMessage(`{"data":null,"nextCursor":null}`)}}, outcome: sessioncatalog.OutcomeFailed},
		{name: "absent data fails", script: fakeCodexScript{Pages: map[string]json.RawMessage{"": json.RawMessage(`{"nextCursor":null}`)}}, outcome: sessioncatalog.OutcomeFailed},
		{name: "malformed row fails", script: fakeCodexScript{Pages: map[string]json.RawMessage{"": json.RawMessage(`{"data":[{"id":"","model":""}],"nextCursor":null}`)}}, outcome: sessioncatalog.OutcomeFailed},
		{name: "repeated cursor fails", script: fakeCodexScript{Pages: map[string]json.RawMessage{"": catalogPage("again", "a"), "again": catalogPage("again", "b")}}, outcome: sessioncatalog.OutcomeFailed},
		{name: "page bound fails", script: fakeCodexScript{Pages: manyPages}, outcome: sessioncatalog.OutcomeFailed},
		{name: "choice bound fails", script: fakeCodexScript{Pages: map[string]json.RawMessage{"": catalogPage("", many...)}}, outcome: sessioncatalog.OutcomeFailed},
		{name: "protocol error fails", script: fakeCodexScript{Pages: map[string]json.RawMessage{}}, outcome: sessioncatalog.OutcomeFailed},
		{name: "unqualified runtime version is unsupported", script: fakeCodexScript{UserAgent: "kandev/0.166.0 (fixture)", Pages: map[string]json.RawMessage{"": catalogPage("", "x")}}, outcome: sessioncatalog.OutcomeUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := startCatalogProbeFixture(t, fakeCodexScript{Pages: map[string]json.RawMessage{"": catalogPage("", "live")}})
			writeFakeCodexScript(t, fixture.dir, tc.script)
			reply, err := fixture.discover(context.Background())
			if err != nil {
				t.Fatalf("discover: %v", err)
			}
			if reply.Result.Outcome != tc.outcome {
				t.Fatalf("outcome = %q, want %q (%+v)", reply.Result.Outcome, tc.outcome, reply.Result)
			}
			if err := sessioncatalog.ValidateDiscovery(&reply.Result); err != nil {
				t.Fatalf("result does not validate: %v", err)
			}
			if tc.outcome != sessioncatalog.OutcomeComplete && (len(reply.Result.Models.Choices) != 0 || reply.Result.Models.Supported) {
				t.Fatalf("unsuccessful result advertised choices: %+v", reply.Result.Models)
			}
			if tc.wantEmpty && (reply.Result.Models.Choices == nil || len(reply.Result.Models.Choices) != 0 || !reply.Result.Models.Supported) {
				t.Fatalf("explicit empty surface = %+v, want supported empty choices", reply.Result.Models)
			}
		})
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryRejectsUnprovenOwner(t *testing.T) {
	fixture := startCatalogProbeFixture(t, fakeCodexScript{Pages: map[string]json.RawMessage{"": catalogPage("", "live")}})
	mutate := map[string]func(*sessioncatalog.TaskProbeRequest){
		"zero generation is not a wildcard": func(r *sessioncatalog.TaskProbeRequest) { r.ProcessGeneration = 0 },
		"another manager":                   func(r *sessioncatalog.TaskProbeRequest) { r.ManagerID = "other-manager" },
		"stale generation":                  func(r *sessioncatalog.TaskProbeRequest) { r.ProcessGeneration++ },
		"another native session":            func(r *sessioncatalog.TaskProbeRequest) { r.SessionID = "other-thread" },
		"stale session source":              func(r *sessioncatalog.TaskProbeRequest) { r.SessionSource++ },
		"zero session source":               func(r *sessioncatalog.TaskProbeRequest) { r.SessionSource = 0 },
	}
	for name, change := range mutate {
		t.Run(name, func(t *testing.T) {
			request := fixture.request
			change(&request)
			_, err := fixture.mgr.DiscoverCatalogCandidates(context.Background(), catalogProbeTestInstance, request)
			if !errors.Is(err, sessioncatalog.ErrTaskProbeOwner) && !errors.Is(err, sessioncatalog.ErrTaskProbeRequest) {
				t.Fatalf("error = %v, want owner/request rejection", err)
			}
		})
	}
	t.Run("missing sealed launch", func(t *testing.T) {
		sealed := fixture.mgr.catalogLaunch.Swap(nil)
		t.Cleanup(func() { fixture.mgr.catalogLaunch.Store(sealed) })
		_, err := fixture.discover(context.Background())
		if !errors.Is(err, sessioncatalog.ErrTaskProbeOwner) {
			t.Fatalf("error = %v, want unavailable owner evidence", err)
		}
	})
	frames, err := fakeCodexFrames(fixture.dir)
	if err != nil {
		t.Fatalf("frames: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("rejected requests launched helpers: %v", frames)
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.7
func TestSessionCatalogTaskDiscoveryOwnerChangeDuringHelper(t *testing.T) {
	changes := map[string]func(*testing.T, *catalogProbeFixture){
		"same-process reload": func(t *testing.T, f *catalogProbeFixture) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = f.mgr.GetAdapter().LoadSession(ctx, f.request.SessionID, nil)
			if SessionCatalogSource(f.mgr.GetAdapter()) == f.request.SessionSource {
				t.Fatal("reload did not advance the session source")
			}
		},
		"replacement process generation": func(t *testing.T, f *catalogProbeFixture) {
			f.mgr.AdvanceProcessGenerationForTest()
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			fixture := startCatalogProbeFixture(t, fakeCodexScript{Pages: map[string]json.RawMessage{"": catalogPage("", "live")}})
			gate := filepath.Join(fixture.dir, "release")
			writeFakeCodexScript(t, fixture.dir, fakeCodexScript{Gate: gate, Pages: map[string]json.RawMessage{"": catalogPage("", "fresh")}})
			done := make(chan error, 1)
			go func() {
				_, err := fixture.discover(context.Background())
				done <- err
			}()
			waitForCatalogFile(t, filepath.Join(fixture.dir, "hanging-*.txt"))
			change(t, fixture)
			if err := os.WriteFile(gate, []byte("1"), 0o600); err != nil {
				t.Fatalf("release gate: %v", err)
			}
			select {
			case err := <-done:
				if !errors.Is(err, sessioncatalog.ErrTaskProbeOwner) {
					t.Fatalf("error = %v, want a changed-owner rejection", err)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("discover did not finish")
			}
		})
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.8
func TestSessionCatalogTaskDiscoveryCancellationKillsOnlyHelperGroup(t *testing.T) {
	fixture := startCatalogProbeFixture(t, fakeCodexScript{Pages: map[string]json.RawMessage{"": catalogPage("", "live")}})
	writeFakeCodexScript(t, fixture.dir, fakeCodexScript{Hang: true, Grandchild: true, Pages: map[string]json.RawMessage{"": catalogPage("", "fresh")}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := fixture.discover(ctx)
		done <- err
	}()
	marker := waitForCatalogFile(t, filepath.Join(fixture.dir, "grandchild-*.txt"))
	waitForCatalogFile(t, filepath.Join(fixture.dir, "hanging-*.txt"))
	helper := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(marker), "grandchild-"), ".txt")
	raw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read grandchild: %v", err)
	}
	grandchild, _ := strconv.Atoi(string(raw))
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want request cancellation", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("cancelled discover did not return")
	}
	helperPID, _ := strconv.Atoi(helper)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && (!processExited(helperPID) || !processExited(grandchild)) {
		time.Sleep(20 * time.Millisecond)
	}
	if !processExited(helperPID) || !processExited(grandchild) {
		t.Fatal("cancellation must kill and reap the whole helper group")
	}
	if fixture.mgr.Status() != StatusRunning || processExited(fixture.livePID) {
		t.Fatal("cancellation must not stop the live agent")
	}
	if fixture.mgr.catalogProbeActive.Load() {
		t.Fatal("cancelled helper must release admission")
	}
}

// @covers AC-AGENTS-SESSION-CATALOG-001.8
func TestSessionCatalogTaskDiscoveryAdmitsOneHelperPerInstance(t *testing.T) {
	fixture := startCatalogProbeFixture(t, fakeCodexScript{Pages: map[string]json.RawMessage{"": catalogPage("", "live")}})
	gate := filepath.Join(fixture.dir, "release")
	writeFakeCodexScript(t, fixture.dir, fakeCodexScript{Gate: gate, Pages: map[string]json.RawMessage{"": catalogPage("", "fresh")}})
	done := make(chan error, 1)
	go func() {
		_, err := fixture.discover(context.Background())
		done <- err
	}()
	waitForCatalogFile(t, filepath.Join(fixture.dir, "hanging-*.txt"))
	if _, err := fixture.discover(context.Background()); !errors.Is(err, ErrCatalogProbeBusy) {
		t.Fatalf("second helper error = %v, want busy", err)
	}
	if err := os.WriteFile(gate, []byte("1"), 0o600); err != nil {
		t.Fatalf("release gate: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("first helper: %v", err)
	}
}
