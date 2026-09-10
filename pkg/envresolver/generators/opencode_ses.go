package generators

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	opencodeSesName    = "opencode-ses"
	opencodeSesPrefix  = "ses"
	opencodeSesTotal   = 26 // chars after the underscore (12 hex + 14 base62)
	opencodeSesHexLen  = 12 // timestamp+counter encoded as hex
	opencodeSesRandLen = 14 // random base62 chars
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func init() {
	Register(&opencodeSesGenerator{})
}

// opencodeSesGenerator produces session IDs matching the OpenCode format:
// {prefix}_XXXXXXXXXXXXYYYYYYYYYYYYYY
// where X = 12 hex chars (timestamp+counter) and Y = 14 base62 chars.
type opencodeSesGenerator struct {
	mu            sync.Mutex
	lastTimestamp int64
	counter       int
}

func (g *opencodeSesGenerator) Name() string {
	return opencodeSesName
}

func (g *opencodeSesGenerator) Generate() string {
	return g.generateWithPrefix(opencodeSesPrefix)
}

func (g *opencodeSesGenerator) generateWithPrefix(prefix string) string {
	now := time.Now().UnixMilli()

	g.mu.Lock()
	if now != g.lastTimestamp {
		g.lastTimestamp = now
		g.counter = 0
	}
	g.counter++
	counter := g.counter
	g.mu.Unlock()

	// Encode timestamp*0x1000 + counter into 6 bytes, matching OpenCode's BigInt encoding.
	encoded := int64(now)*0x1000 + int64(counter)

	timeBytes := make([]byte, 6)
	for i := 0; i < 6; i++ {
		timeBytes[i] = byte((encoded >> uint(40-8*i)) & 0xff)
	}

	randBytes := make([]byte, opencodeSesRandLen)
	if _, err := rand.Read(randBytes); err != nil {
		// Fallback: use timestamp-based pseudo-random (never happens in practice).
		for i := range randBytes {
			randBytes[i] = byte(now >> uint(i%8*8))
		}
	}

	randPart := make([]byte, opencodeSesRandLen)
	for i := range randPart {
		randPart[i] = base62Chars[randBytes[i]%62]
	}

	return prefix + "_" + hex.EncodeToString(timeBytes) + string(randPart)
}
