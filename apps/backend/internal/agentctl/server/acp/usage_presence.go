package acp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"

	sdk "github.com/coder/acp-go-sdk"
)

const inputMissingMetaKey = "_kandevInputTokensMissing"
const outputMissingMetaKey = "_kandevOutputTokensMissing"
const maxUsageFrameBytes = 64 * 1024 * 1024

// PromptUsageTokenPresence distinguishes SDK scalar zeros from missing/null
// required token counts observed before typed decoding.
func PromptUsageTokenPresence(usage *sdk.Usage) (inputPresent, outputPresent bool) {
	inputMissing, _ := usage.Meta[inputMissingMetaKey].(bool)
	outputMissing, _ := usage.Meta[outputMissingMetaKey].(bool)
	return !inputMissing, !outputMissing
}

type usagePresenceReader struct {
	reader  *bufio.Reader
	pending []byte
	err     error
}

func (r *usagePresenceReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		r.pending, r.err = r.readFrame()
		r.pending = preserveTokenPresence(r.pending)
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	if n == 0 {
		return 0, r.err
	}
	return n, nil
}

func (r *usagePresenceReader) readFrame() ([]byte, error) {
	frame, err := r.reader.ReadSlice('\n')
	if err != bufio.ErrBufferFull {
		return frame, err
	}
	frame = append([]byte(nil), frame...)
	for err == bufio.ErrBufferFull {
		var part []byte
		part, err = r.reader.ReadSlice('\n')
		if len(frame)+len(part) > maxUsageFrameBytes {
			return nil, fmt.Errorf("ACP frame exceeds %d bytes", maxUsageFrameBytes)
		}
		frame = append(frame, part...)
	}
	return frame, err
}

func mayContainPromptUsage(frame []byte) bool {
	return (bytes.Contains(frame, []byte(`"usage"`)) && bytes.Contains(frame, []byte(`"result"`))) ||
		bytes.Contains(frame, []byte(`\u`))
}

// Other frames and fully measured usage pass through without JSON rewriting.
// The SDK's scalar fields cannot themselves distinguish missing counts from zero.
func preserveTokenPresence(frame []byte) []byte {
	if !mayContainPromptUsage(frame) {
		return frame
	}
	var envelope, result, usage map[string]json.RawMessage
	if json.Unmarshal(frame, &envelope) != nil || json.Unmarshal(envelope["result"], &result) != nil ||
		json.Unmarshal(result["usage"], &usage) != nil || usage == nil {
		return frame
	}
	inputMissing := tokenCountMissing(usage["inputTokens"])
	outputMissing := tokenCountMissing(usage["outputTokens"])
	var meta map[string]json.RawMessage
	if raw := usage["_meta"]; len(raw) != 0 {
		if json.Unmarshal(raw, &meta) != nil {
			return frame
		}
	}
	if !inputMissing && !outputMissing {
		_, inputMarked := meta[inputMissingMetaKey]
		_, outputMarked := meta[outputMissingMetaKey]
		if !inputMarked && !outputMarked {
			return frame
		}
	}
	if meta == nil {
		meta = make(map[string]json.RawMessage, 2)
	}
	setTokenMissingMarker(meta, inputMissingMetaKey, inputMissing)
	setTokenMissingMarker(meta, outputMissingMetaKey, outputMissing)
	usage["_meta"], _ = json.Marshal(meta)
	result["usage"], _ = json.Marshal(usage)
	envelope["result"], _ = json.Marshal(result)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return frame
	}
	return append(encoded, '\n')
}

func tokenCountMissing(raw json.RawMessage) bool {
	value := bytes.TrimSpace(raw)
	return len(value) == 0 || bytes.Equal(value, []byte("null"))
}

func setTokenMissingMarker(meta map[string]json.RawMessage, key string, missing bool) {
	if missing {
		meta[key] = json.RawMessage("true")
	} else {
		delete(meta, key)
	}
}
