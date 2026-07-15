package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"

	"github.com/gen2brain/malgo"
)

// Engine owns a single malgo context shared by the recorder and player.
type Engine struct {
	ctx *malgo.AllocatedContext
}

func NewEngine() (*Engine, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(string) {})
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}
	return &Engine{ctx: ctx}, nil
}

func (e *Engine) Close() {
	if e.ctx != nil {
		_ = e.ctx.Uninit()
		e.ctx.Free()
	}
}

// ---- Recorder ---------------------------------------------------------

// Recorder captures mono s16 PCM at the configured sample rate. Start/Stop
// are cheap; the device is created on Start and torn down on Stop.
type Recorder struct {
	engine     *Engine
	sampleRate uint32

	mu     sync.Mutex
	buf    bytes.Buffer
	device *malgo.Device
}

func NewRecorder(e *Engine, sampleRate uint32) *Recorder {
	return &Recorder{engine: e, sampleRate: sampleRate}
}

func (r *Recorder) Start() error {
	r.mu.Lock()
	r.buf.Reset()
	r.mu.Unlock()

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = 1
	cfg.SampleRate = r.sampleRate
	cfg.Alsa.NoMMap = 1

	onData := func(_, in []byte, _ uint32) {
		r.mu.Lock()
		r.buf.Write(in)
		r.mu.Unlock()
	}
	dev, err := malgo.InitDevice(r.engine.ctx.Context, cfg, malgo.DeviceCallbacks{Data: onData})
	if err != nil {
		return fmt.Errorf("init capture device: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start capture: %w", err)
	}
	r.device = dev
	return nil
}

// Stop ends capture and returns the recording wrapped as a WAV blob.
func (r *Recorder) Stop() []byte {
	if r.device != nil {
		_ = r.device.Stop()
		r.device.Uninit()
		r.device = nil
	}
	r.mu.Lock()
	pcm := make([]byte, r.buf.Len())
	copy(pcm, r.buf.Bytes())
	r.mu.Unlock()
	return EncodeWAV(pcm, r.sampleRate, 1)
}

// ---- Player -----------------------------------------------------------

// Player plays a WAV blob. Play blocks until playback finishes or ctx is
// cancelled (barge-in). Cancellation stops the device early. While playing it
// exposes recent amplitude via Levels for the waveform visualizer.
type Player struct {
	engine *Engine

	mu     sync.Mutex
	curPCM []byte
	curPos int
}

func NewPlayer(e *Engine) *Player { return &Player{engine: e} }

func (p *Player) Play(ctx context.Context, wav []byte) error {
	pcm, sampleRate, channels, err := DecodeWAV(wav)
	if err != nil {
		return err
	}
	// Prepend a short lead-in of silence so the audio device's cold-start
	// warmup swallows silence instead of the first words. Tunable via
	// VOX_LEADIN_MS (default 300ms).
	if lead := leadInBytes(sampleRate, channels); lead > 0 {
		pcm = append(make([]byte, lead), pcm...)
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = uint32(channels)
	cfg.SampleRate = sampleRate
	cfg.Alsa.NoMMap = 1

	p.mu.Lock()
	p.curPCM = pcm
	p.curPos = 0
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.curPCM = nil
		p.curPos = 0
		p.mu.Unlock()
	}()

	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	onData := func(out, _ []byte, frameCount uint32) {
		need := int(frameCount) * channels * 2 // s16 = 2 bytes/sample
		p.mu.Lock()
		pos := p.curPos
		n := copy(out, pcm[pos:min(pos+need, len(pcm))])
		p.curPos += n
		reachedEnd := p.curPos >= len(pcm)
		p.mu.Unlock()
		// Zero-fill the remainder of the buffer to avoid clicks.
		for i := n; i < len(out); i++ {
			out[i] = 0
		}
		if reachedEnd {
			finish()
		}
	}

	dev, err := malgo.InitDevice(p.engine.ctx.Context, cfg, malgo.DeviceCallbacks{Data: onData})
	if err != nil {
		return fmt.Errorf("init playback device: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start playback: %w", err)
	}
	defer func() {
		_ = dev.Stop()
		dev.Uninit()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ---- WAV --------------------------------------------------------------

// EncodeWAV wraps raw s16 PCM in a canonical 44-byte WAV header.
func EncodeWAV(pcm []byte, sampleRate uint32, channels int) []byte {
	var b bytes.Buffer
	dataLen := uint32(len(pcm))
	byteRate := sampleRate * uint32(channels) * 2
	blockAlign := uint16(channels * 2)

	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+dataLen))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))       // subchunk size
	binary.Write(&b, binary.LittleEndian, uint16(1))        // PCM
	binary.Write(&b, binary.LittleEndian, uint16(channels)) //
	binary.Write(&b, binary.LittleEndian, sampleRate)
	binary.Write(&b, binary.LittleEndian, byteRate)
	binary.Write(&b, binary.LittleEndian, blockAlign)
	binary.Write(&b, binary.LittleEndian, uint16(16)) // bits per sample
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, dataLen)
	b.Write(pcm)
	return b.Bytes()
}

// DecodeWAV extracts PCM + format from a simple PCM WAV. It scans chunks so
// it tolerates the extra metadata chunks some encoders emit.
func DecodeWAV(wav []byte) (pcm []byte, sampleRate uint32, channels int, err error) {
	if len(wav) < 44 || string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("not a WAV file")
	}
	p := 12
	for p+8 <= len(wav) {
		id := string(wav[p : p+4])
		size := int(binary.LittleEndian.Uint32(wav[p+4 : p+8]))
		body := p + 8
		if body+size > len(wav) {
			size = len(wav) - body
		}
		switch id {
		case "fmt ":
			channels = int(binary.LittleEndian.Uint16(wav[body+2 : body+4]))
			sampleRate = binary.LittleEndian.Uint32(wav[body+4 : body+8])
		case "data":
			pcm = wav[body : body+size]
		}
		p = body + size
		if size%2 == 1 {
			p++ // chunks are word-aligned
		}
	}
	if pcm == nil || channels == 0 {
		return nil, 0, 0, fmt.Errorf("no PCM data found in WAV")
	}
	return pcm, sampleRate, channels, nil
}

// Levels returns n amplitude bars (0..1) from the most recent ~100ms of
// captured audio, for the mic waveform.
func (r *Recorder) Levels(n int) []float64 {
	r.mu.Lock()
	b := r.buf.Bytes()
	win := int(r.sampleRate) / 10 * 2 // 100ms of s16 mono, in bytes
	if win > len(b) {
		win = len(b)
	}
	tail := b[len(b)-win:]
	cp := make([]byte, len(tail))
	copy(cp, tail)
	r.mu.Unlock()
	return levelsFromPCM(cp, n)
}

// Levels returns n amplitude bars (0..1) from the audio around the current
// playback position, for the Claude waveform.
func (p *Player) Levels(n int) []float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.curPCM) == 0 {
		return make([]float64, n)
	}
	const win = 9600 // ~100ms @ 24kHz s16 mono, in bytes
	start := p.curPos - win
	if start < 0 {
		start = 0
	}
	end := p.curPos
	if end > len(p.curPCM) {
		end = len(p.curPCM)
	}
	seg := p.curPCM[start:end]
	cp := make([]byte, len(seg))
	copy(cp, seg)
	return levelsFromPCM(cp, n)
}

// levelsFromPCM splits s16 mono PCM into n segments and returns the RMS of
// each, normalized and gained into 0..1.
func levelsFromPCM(pcm []byte, n int) []float64 {
	out := make([]float64, n)
	samples := len(pcm) / 2
	if samples == 0 || n == 0 {
		return out
	}
	per := samples / n
	if per == 0 {
		per = 1
	}
	for i := 0; i < n; i++ {
		start := i * per
		end := start + per
		if start >= samples {
			break
		}
		if end > samples {
			end = samples
		}
		var sum float64
		cnt := 0
		for s := start; s < end; s++ {
			v := int16(binary.LittleEndian.Uint16(pcm[s*2:]))
			f := float64(v) / 32768.0
			sum += f * f
			cnt++
		}
		if cnt > 0 {
			rms := math.Sqrt(sum / float64(cnt))
			out[i] = math.Min(1.0, rms*4.0) // gain so speech reads visibly
		}
	}
	return out
}

// leadInBytes returns the size of the playback lead-in silence in bytes.
func leadInBytes(sampleRate uint32, channels int) int {
	ms := 300
	if v := os.Getenv("VOX_LEADIN_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			ms = n
		}
	}
	return int(sampleRate) * channels * 2 * ms / 1000
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
