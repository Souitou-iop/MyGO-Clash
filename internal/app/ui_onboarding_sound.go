package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
)

// The guide's sound: each scene has a short score of its own, made from
// the scene's own timing (ui_onboarding_score.go), so that what sounds
// falls as what moves, over a soft chord; the chords go round the key of A
// as the pages turn, and come home with the chime as the compass lands
// back on the sidebar's mark. The scores are made in memory and played by
// mygo's audio, which starts them within a few hundredths of a second of
// being asked, so that they keep time with the drawing; one giving way
// fades out rather than stopping. The guide is quiet when muted, when
// skipped, and on the Mac while the system's interface sounds are off.

const (
	sndRate = 44100
	// A scene's score rings on sndTail seconds past its scene, its chord
	// and last notes dying away, and is let go over its last sndRelease
	// seconds: it fades out as music does, rather than stopping.
	sndTail    = 2.6
	sndRelease = 2.0
)

// encodeWAV is samples as 16-bit mono WAV, their loudest at peak, let go
// over their last release seconds so that they die away into silence.
func encodeWAV(samples []float64, peak, release float64) []byte {
	top := 1e-9
	for _, v := range samples {
		top = max(top, math.Abs(v))
	}
	gain := peak / top
	n := len(samples)
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+2*n))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, struct {
		Size                   uint32
		Format, Channels       uint16
		Rate, ByteRate         uint32
		BlockAlign, BitsSample uint16
	}{16, 1, 1, sndRate, 2 * sndRate, 2, 16})
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(2*n))
	fade := max(1, int(release*sndRate))
	for i, v := range samples {
		v *= gain
		if left := n - i; left < fade {
			// A curve that leaves as gently as it lands: no corner where
			// the fade begins, nor where it ends.
			g := math.Sin(math.Pi / 2 * float64(left) / float64(fade))
			v *= g * g
		}
		_ = binary.Write(&b, binary.LittleEndian, int16(math.Round(v*32767)))
	}
	return b.Bytes()
}

// onbAudio plays the guide's scores, one at a time.
type onbAudio struct {
	mu     sync.Mutex
	gen    int           // the score asked for last
	cur    *audio.Player // the one playing
	muted  bool
	quiet  bool // the system's interface sounds are off
	sounds map[string]*audio.Sound
}

var onbSound = &onbAudio{}

// sound is the score called name, made and decoded the first time.
func (au *onbAudio) sound(name string) *audio.Sound {
	au.mu.Lock()
	snd := au.sounds[name]
	au.mu.Unlock()
	if snd != nil {
		return snd
	}
	wav := renderScore(name)
	if wav == nil {
		return nil
	}
	snd, err := audio.DecodeSound(wav)
	if err != nil {
		return nil
	}
	au.mu.Lock()
	if au.sounds == nil {
		au.sounds = map[string]*audio.Sound{}
	}
	au.sounds[name] = snd
	au.mu.Unlock()
	return snd
}

// fadeOut turns a player down to silence over d, then stops it.
func fadeOut(p *audio.Player, d time.Duration) {
	if p == nil {
		return
	}
	go func() {
		from := p.Volume()
		const steps = 24
		for i := 1; i <= steps; i++ {
			k := 1 - float64(i)/steps
			p.SetVolume(from * k * k)
			time.Sleep(d / steps)
		}
		p.Stop()
		p.Close()
	}()
}

// onbScore plays a scene's score; the one before fades out under it.
func (m *mainUI) onbScore(name string, still bool) {
	if still {
		name += "-still"
	}
	au := onbSound
	au.mu.Lock()
	au.gen++
	gen, muted := au.gen, au.muted
	au.mu.Unlock()
	if muted {
		return
	}
	go func() {
		snd := au.sound(name)
		if snd == nil {
			return
		}
		au.mu.Lock()
		if gen != au.gen || au.muted || au.quiet {
			au.mu.Unlock()
			return // another page came meanwhile
		}
		prev := au.cur
		pl := snd.NewPlayer().SetVolume(0.6)
		pl.Play()
		au.cur = pl
		au.mu.Unlock()
		// The page before rings on under the new one's first notes, and its
		// chord coming in, as it fades.
		fadeOut(prev, 450*time.Millisecond)
	}()
}

// onbPrepare makes the scores ahead, as the guide opens, so that each page
// sounds as soon as it comes; and clears the files an earlier version
// wrote for the system's player.
func (m *mainUI) onbPrepare(still bool) {
	go func() {
		q := !uiSoundsOn()
		onbSound.mu.Lock()
		onbSound.quiet = q
		onbSound.mu.Unlock()
		if m.a != nil && m.a.dirs.Cache != "" {
			old, _ := filepath.Glob(filepath.Join(m.a.dirs.Cache, "onboarding-*.wav"))
			for _, f := range old {
				_ = os.Remove(f)
			}
		}
		for _, name := range onbScoreNames {
			if still {
				name += "-still"
			}
			onbSound.sound(name)
		}
		onbSound.sound("chime")
	}()
}

// onbHush fades out the score playing, after a while.
func onbHush(after time.Duration) {
	au := onbSound
	au.mu.Lock()
	au.gen++
	cur := au.cur
	au.cur = nil
	au.mu.Unlock()
	if cur != nil {
		time.AfterFunc(after, func() { fadeOut(cur, 220*time.Millisecond) })
	}
}

// onbMute mutes the guide, or lets it sound again from the next page.
func onbMute(muted bool) {
	onbSound.mu.Lock()
	onbSound.muted = muted
	onbSound.mu.Unlock()
	if muted {
		onbHush(0)
	}
}

func onbMuted() bool {
	onbSound.mu.Lock()
	defer onbSound.mu.Unlock()
	return onbSound.muted
}

// onbChime plays the last sound as the compass flies home, the scene's
// score giving way to it: it starts so that what it has for the landing
// falls as the compass lands, the output's own lead included.
func (m *mainUI) onbChime() {
	if onbMuted() {
		onbHush(150 * time.Millisecond)
		return
	}
	lead := time.Duration((onbFly - sndLead) * float32(time.Second))
	onbHush(lead)
	snd := onbSound.sound("chime")
	onbSound.mu.Lock()
	quiet := onbSound.quiet
	onbSound.mu.Unlock()
	if snd == nil || quiet {
		return
	}
	time.AfterFunc(lead, func() {
		if !onbMuted() {
			snd.NewPlayer().SetVolume(0.8).Play()
		}
	})
}

// uiSoundsOn reports whether the system plays interface sounds: the Mac's
// Sound settings say; elsewhere they are taken to be on.
func uiSoundsOn() bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "com.apple.sound.uiaudio.enabled").Output()
	return err != nil || strings.TrimSpace(string(out)) != "0"
}

// playWAV plays a WAV to its end, for listening to the scores in a test.
func playWAV(wav []byte) error {
	snd, err := audio.DecodeSound(wav)
	if err != nil {
		return err
	}
	done := make(chan struct{})
	p := snd.NewPlayer().SetVolume(0.8)
	p.OnEnd(func() { close(done) })
	p.Play()
	select {
	case <-done:
	case <-time.After(snd.Duration() + 2*time.Second):
	}
	return audio.Err()
}
