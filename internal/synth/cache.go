package synth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/nlink-jp/voice-studio-mcp/internal/workspace"
)

// cacheIndexRel is the cache index location inside a workspace.
const cacheIndexRel = "cache/index.json"

// CacheEntry records one synthesized line.
type CacheEntry struct {
	Hash            string  `json:"hash"`
	DurationSeconds float64 `json:"duration_seconds"`
	SynthesizedAt   string  `json:"synthesized_at"` // RFC3339
}

// CacheKey derives the content hash of one line's synthesis inputs.
//
// pause_after_ms is deliberately NOT part of the key: pauses are applied at
// mastering time, so changing a pause must not force re-synthesis. The engine
// version is included because a model/engine update can change output.
func CacheKey(engineVersion, speakerUUID string, styleID int, speed, intensity, volume, prePhoneme, postPhoneme float64, samplingRate int, text string) string {
	payload := "v2|" + engineVersion +
		"|" + speakerUUID +
		"|" + strconv.Itoa(styleID) +
		"|" + formatFloat(speed) +
		"|" + formatFloat(intensity) +
		"|" + formatFloat(volume) +
		"|" + formatFloat(prePhoneme) +
		"|" + formatFloat(postPhoneme) +
		"|" + strconv.Itoa(samplingRate) +
		"|" + text
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// CacheStore is the on-disk synthesis cache index of one workspace. All I/O
// goes through the workspace's os.Root containment (ADR-0010). Safe for
// concurrent use; every Put persists atomically so an interrupted batch
// loses at most the in-flight line.
type CacheStore struct {
	ws *workspace.Workspace

	mu  sync.Mutex
	idx map[string]CacheEntry // key: line id (decimal string)
}

// OpenCacheStore loads (or initializes) the workspace's cache index.
func OpenCacheStore(ws *workspace.Workspace) (*CacheStore, error) {
	s := &CacheStore{ws: ws, idx: map[string]CacheEntry{}}
	b, err := ws.ReadFile(cacheIndexRel)
	if err != nil {
		// Missing or unreadable index only costs re-synthesis; start fresh.
		return s, nil
	}
	if err := json.Unmarshal(b, &s.idx); err != nil {
		s.idx = map[string]CacheEntry{}
	}
	return s, nil
}

// Lookup reports a cache hit for lineID: the stored hash matches and the
// WAV file still exists inside the workspace.
func (s *CacheStore) Lookup(lineID int, hash string) (CacheEntry, bool) {
	s.mu.Lock()
	e, ok := s.idx[strconv.Itoa(lineID)]
	s.mu.Unlock()
	if !ok || e.Hash != hash {
		return CacheEntry{}, false
	}
	if _, err := s.ws.Stat(WavRel(lineID)); err != nil {
		return CacheEntry{}, false
	}
	return e, true
}

// Put records a synthesized line and persists the index.
func (s *CacheStore) Put(lineID int, e CacheEntry) error {
	if e.SynthesizedAt == "" {
		e.SynthesizedAt = time.Now().Format(time.RFC3339)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idx[strconv.Itoa(lineID)] = e
	b, err := json.MarshalIndent(s.idx, "", "  ")
	if err != nil {
		return err
	}
	return s.ws.WriteFileAtomic(cacheIndexRel, b)
}
