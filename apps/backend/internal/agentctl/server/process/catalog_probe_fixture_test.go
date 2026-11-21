package process

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// fakeCodexAppServerEnv activates a process-level native app-server fixture.
// Its value is a directory holding script.json; every process records its
// received frames, environment, argv and cwd there, keyed by PID. Like the
// real provider, a process reads its catalog script once at startup.
const fakeCodexAppServerEnv = "KANDEV_TEST_FAKE_CODEX_APP_SERVER"

type fakeCodexScript struct {
	UserAgent  string                     `json:"user_agent"`
	Pages      map[string]json.RawMessage `json:"pages"`
	Hang       bool                       `json:"hang"`
	Grandchild bool                       `json:"grandchild"`
	Gate       string                     `json:"gate"`
}

type fakeCodexFrame struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func runFakeCodexAppServer(dir string) {
	pid := strconv.Itoa(os.Getpid())
	script := fakeCodexScript{UserAgent: "kandev/0.169.0 (fixture)"}
	if data, err := os.ReadFile(filepath.Join(dir, "script.json")); err == nil {
		_ = json.Unmarshal(data, &script)
	}
	if script.UserAgent == "" {
		script.UserAgent = "kandev/0.169.0 (fixture)"
	}
	environment, _ := json.Marshal(os.Environ())
	_ = os.WriteFile(filepath.Join(dir, "env-"+pid+".json"), environment, 0o600)
	cwd, _ := os.Getwd()
	_ = os.WriteFile(filepath.Join(dir, "cwd-"+pid+".txt"), []byte(cwd), 0o600)
	argv, _ := json.Marshal(os.Args)
	_ = os.WriteFile(filepath.Join(dir, "args-"+pid+".json"), argv, 0o600)
	frames, err := os.OpenFile(filepath.Join(dir, "frames-"+pid+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(3)
	}
	defer func() { _ = frames.Close() }()
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 1<<20), 4<<20)
	writer := bufio.NewWriter(os.Stdout)
	for reader.Scan() {
		var frame fakeCodexFrame
		if json.Unmarshal(reader.Bytes(), &frame) != nil {
			continue
		}
		_, _ = frames.Write(append(append([]byte(nil), reader.Bytes()...), '\n'))
		_ = frames.Sync()
		if len(frame.ID) == 0 {
			continue
		}
		result, failure := fakeCodexResult(dir, pid, &script, frame)
		var payload []byte
		if failure != "" {
			payload, _ = json.Marshal(map[string]any{"id": frame.ID, "error": map[string]any{"code": -32601, "message": failure}})
		} else {
			payload, _ = json.Marshal(map[string]any{"id": frame.ID, "result": result})
		}
		_, _ = writer.Write(append(payload, '\n'))
		_ = writer.Flush()
	}
}

func fakeCodexResult(dir, pid string, script *fakeCodexScript, frame fakeCodexFrame) (any, string) {
	switch frame.Method {
	case "initialize":
		return map[string]any{"userAgent": script.UserAgent, "codexHome": "", "platformOs": "linux", "platformFamily": "unix"}, ""
	case "thread/start":
		return map[string]any{"thread": map[string]any{"id": "fixture-thread"}, "model": "live-model"}, ""
	case "model/list":
		return fakeCodexModelPage(dir, pid, script, frame.Params)
	default:
		return nil, "unsupported fixture method " + frame.Method
	}
}

func fakeCodexModelPage(dir, pid string, script *fakeCodexScript, raw json.RawMessage) (any, string) {
	if script.Hang {
		fakeCodexHang(dir, pid, script.Grandchild)
	}
	if script.Gate != "" {
		fakeCodexWait(dir, pid, script.Gate)
	}
	var params struct {
		Cursor *string `json:"cursor"`
	}
	_ = json.Unmarshal(raw, &params)
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	page, ok := script.Pages[cursor]
	if !ok {
		return nil, "unknown cursor"
	}
	return page, ""
}

func fakeCodexHang(dir, pid string, grandchild bool) {
	if grandchild {
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), kandevTestFixtureEnv+"=sleep 60")
		if err := child.Start(); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "grandchild-"+pid+".txt"), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "hanging-"+pid+".txt"), []byte("1"), 0o600)
	select {}
}

func fakeCodexWait(dir, pid, gate string) {
	_ = os.WriteFile(filepath.Join(dir, "hanging-"+pid+".txt"), []byte("1"), 0o600)
	for {
		if _, err := os.Stat(gate); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fakeCodexFrames returns the method sequence each recorded process received.
func fakeCodexFrames(dir string) (map[string][]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "frames-*.jsonl"))
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pid := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "frames-"), ".jsonl")
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var frame fakeCodexFrame
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				return nil, fmt.Errorf("decode recorded frame: %w", err)
			}
			result[pid] = append(result[pid], frame.Method)
		}
	}
	return result, nil
}
