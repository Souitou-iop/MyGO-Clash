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
	"strconv"
	"strings"
	"sync"
	"time"
)

// The guide's sound: each scene has a short score of its own, made from
// the scene's own timing (ui_onboarding_score.go), so that what sounds
// falls as what moves, over a soft chord; the chords go round the key of A
// as the pages turn, and come home with the chime as the compass lands
// back on the sidebar's mark. MyGo plays no sound of its own: a score is
// written once to the cache and played by the system's player. The guide
// is quiet when muted, when skipped, and on the Mac while the system's
// interface sounds are off.

const (
	sndRate = 44100
	// sndVersion names the files: a new one for a new score.
	sndVersion = 5
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
	mu    sync.Mutex
	gen   int       // the score asked for last
	cur   *exec.Cmd // the one playing
	muted bool
	quiet bool // the system's interface sounds are off
}

var onbSound = &onbAudio{}

// sndDir is where the scores are written; none in a test's window, which
// has no app.
func (m *mainUI) sndDir() string {
	if m.a == nil {
		return ""
	}
	if m.a.dirs.Cache != "" {
		return m.a.dirs.Cache
	}
	return os.TempDir()
}

// sndFile is the score name's file in dir, written if it is not yet.
func sndFile(dir, name string) (string, bool) {
	path := filepath.Join(dir, "onboarding-"+name+"-"+strconv.Itoa(sndVersion)+".wav")
	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	wav := renderScore(name)
	if wav == nil || os.WriteFile(path, wav, 0o644) != nil {
		return "", false
	}
	return path, true
}

// onbScore plays a scene's score: the one before goes a moment after it
// starts, so that the two overlap rather than leave a gap.
func (m *mainUI) onbScore(name string, still bool) {
	dir := m.sndDir()
	if dir == "" {
		return
	}
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
		path, ok := sndFile(dir, name)
		if !ok {
			return
		}
		au.mu.Lock()
		if gen != au.gen || au.muted || au.quiet {
			au.mu.Unlock()
			return // another page came meanwhile
		}
		prev := au.cur
		cmd := soundCmd(path, 0.6)
		if cmd == nil || cmd.Start() != nil {
			au.mu.Unlock()
			return
		}
		au.cur = cmd
		au.mu.Unlock()
		if prev != nil {
			// The page before rings on under the new one's first notes, and
			// its chord coming in, before it goes.
			time.AfterFunc(400*time.Millisecond, func() { stopCmd(prev) })
		}
		_ = cmd.Wait()
		au.mu.Lock()
		if au.cur == cmd {
			au.cur = nil
		}
		au.mu.Unlock()
	}()
}

// onbPrepare writes the scores ahead, as the guide opens, so that each
// page sounds as it comes.
func (m *mainUI) onbPrepare(still bool) {
	dir := m.sndDir()
	if dir == "" {
		return
	}
	go func() {
		q := !uiSoundsOn()
		onbSound.mu.Lock()
		onbSound.quiet = q
		onbSound.mu.Unlock()
		// The scores of an earlier version go.
		old, _ := filepath.Glob(filepath.Join(dir, "onboarding-*.wav"))
		for _, f := range old {
			if !strings.HasSuffix(f, "-"+strconv.Itoa(sndVersion)+".wav") {
				_ = os.Remove(f)
			}
		}
		for _, name := range onbScoreNames {
			if still {
				name += "-still"
			}
			sndFile(dir, name)
		}
		sndFile(dir, "chime")
	}()
}

// onbHush stops the score playing, after a while.
func onbHush(after time.Duration) {
	au := onbSound
	au.mu.Lock()
	au.gen++
	cur := au.cur
	au.cur = nil
	au.mu.Unlock()
	if cur != nil {
		time.AfterFunc(after, func() { stopCmd(cur) })
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
// score giving way to it.
func (m *mainUI) onbChime() {
	dir := m.sndDir()
	if dir == "" || onbMuted() {
		onbHush(150 * time.Millisecond)
		return
	}
	// It starts so that what it has for the landing falls as the compass
	// lands, the player's own start included.
	lead := time.Duration((onbFly - sndLead) * float32(time.Second))
	onbHush(lead + 60*time.Millisecond)
	go func() {
		time.Sleep(lead)
		if path, ok := sndFile(dir, "chime"); ok {
			_ = playSound(path)
		}
	}()
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

// soundCmd is the system's player playing path at volume (0 to 1, where
// the player has one), or nil where there is none.
func soundCmd(path string, volume float64) *exec.Cmd {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("afplay", "-v", strconv.FormatFloat(volume, 'f', 2, 64), path)
	case "windows":
		q := strings.ReplaceAll(path, "'", "''")
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "(New-Object Media.SoundPlayer '"+q+"').PlaySync()")
	default:
		for _, p := range [][]string{{"paplay"}, {"pw-play"}, {"aplay", "-q"}} {
			if _, err := exec.LookPath(p[0]); err == nil {
				cmd = exec.Command(p[0], append(p[1:], path)...)
				break
			}
		}
	}
	if cmd != nil {
		hideConsole(cmd)
	}
	return cmd
}

// playSound plays a WAV file to its end, if the system can and its
// interface sounds are on.
func playSound(path string) error {
	if !uiSoundsOn() {
		return nil
	}
	cmd := soundCmd(path, 0.8)
	if cmd == nil {
		return nil
	}
	timer := time.AfterFunc(10*time.Second, func() { stopCmd(cmd) })
	defer timer.Stop()
	return cmd.Run()
}

func stopCmd(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
