package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rohanthewiz/cats/wire"
)

// livePicker is the /model picker as a dev catway captured it from Claude Code
// 2.1.318 (N-079), blank rows dropped, pane launched with --model haiku.
const livePicker = ` ▐▛███▛█   Claude Code v2.1.318
▝▜██████▀  Haiku 5.5 · Claude Max
❯ /model
────────────────────────────────────────────────────────────────────────
  Select model
  Switch between Claude models. Your pick becomes the default for new sessions. For other/previous model names,
  specify with --model.
    1.  Default (recommended)  Opus 5.5 · Best for everyday, complex tasks
    2.  Opus 5.5               For complex work and everyday tasks
    3.  Fable 5.1              For your toughest challenges
    4.  Sonnet 5.5             Most efficient for simpler tasks
  ❯ 5.  Haiku 5.5 ✔            Fastest for quick answers
    6.  Haiku 4.5              Fastest for quick answers
  ↓ 7.  Sonnet 5               Efficient for routine tasks
     … +6 models
  ◐ Medium effort (default) ←/→ to adjust
  Use /fast to turn on Fast mode (Opus 5.5).
  Enter to set as default · s to use this session only · Esc to cancel
`

// TestParsePickerLive reads the real capture: rows by number, the ❯ and ✔,
// the hidden rows below, the effort word and the session-only footer.
func TestParsePickerLive(t *testing.T) {
	s := parsePicker(livePicker)
	if !s.open || s.search || !s.sessionOnly || !s.moreBelow {
		t.Fatalf("state = %+v, want open, on the list, session-only offered, more below", s)
	}
	if len(s.rows) != 7 {
		t.Fatalf("rows = %+v, want 7", s.rows)
	}
	f, ok := s.focused()
	if !ok || f.num != 5 || f.label != "Haiku 5.5" || !f.current {
		t.Errorf("focused = %+v, want row 5 Haiku 5.5 ✔", f)
	}
	if s.rows[0].label != "Default (recommended)" || s.rows[6].label != "Sonnet 5" {
		t.Errorf("labels = %q … %q", s.rows[0].label, s.rows[6].label)
	}
	if s.effort != "medium" {
		t.Errorf("effort = %q, want medium", s.effort)
	}
}

// TestParsePickerIgnoresTheConversation: a numbered list and quoted picker
// words above the input box are not a picker. Only a footer on the last line
// drawn opens one, and rows are read only below its header.
func TestParsePickerIgnoresTheConversation(t *testing.T) {
	screen := "  Select model\n  1. Opus\n  Enter to set as default · s to use this session only · Esc to cancel\n" +
		"⏺ That is the picker's footer.\n  1. first\n  2. second\n❯ \n  ? for shortcuts\n"
	if s := parsePicker(screen); s.open {
		t.Errorf("conversation text read as an open picker: %+v", s)
	}
}

// TestPickerLabelFor pins how a prompt's model names a picker row.
func TestPickerLabelFor(t *testing.T) {
	cases := []struct {
		in     string
		label  string
		family bool
	}{
		{"opus", "opus", true},
		{"Sonnet", "sonnet", true},
		{"default", "default", true},
		{"claude-opus-5-5", "opus 5.5", false},
		{"claude-sonnet-4-6", "sonnet 4.6", false},
		{"claude-opus-5", "opus 5", false},
		{"claude-haiku-4-5-20251001", "haiku 4.5", false},
		{"opus[1m]", "opus[1m]", false},
		{"claude-opus-5-5-fast", "claude-opus-5-5-fast", false},
	}
	for _, c := range cases {
		label, family := pickerLabelFor(c.in)
		if label != c.label || family != c.family {
			t.Errorf("pickerLabelFor(%q) = %q, %v; want %q, %v", c.in, label, family, c.label, c.family)
		}
	}
}

// TestPickerRowFor: an alias is its family's first row (the newest), never the
// Default row; an id is its exact label; anything else is no row at all.
func TestPickerRowFor(t *testing.T) {
	rows := simRows()
	var all []pickerRow
	for i, m := range rows {
		all = append(all, pickerRow{num: i + 1, label: m.label})
	}
	cases := map[string]int{
		"opus": 2, "fable": 3, "sonnet": 4, "haiku": 5, "default": 1,
		"claude-sonnet-4-6": 13, "claude-opus-5": 8, "claude-haiku-4-5": 6,
		"opus[1m]": 0, "gpt-5": 0,
	}
	for model, want := range cases {
		r, ok := pickerRowFor(all, model)
		if want == 0 {
			if ok {
				t.Errorf("pickerRowFor(%q) = row %d, want none", model, r.num)
			}
			continue
		}
		if !ok || r.num != want {
			t.Errorf("pickerRowFor(%q) = row %d (%v), want %d", model, r.num, ok, want)
		}
	}
}

// TestSessionOnlyKey pins the keybindings.json reading (N-079 (5)): `s`
// unless the user moved the action, and a refusal whenever the key we would
// press could mean something else.
func TestSessionOnlyKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.json")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if k, err := sessionOnlyKey(path); err != nil || k != (wire.KeyStroke{Text: "s"}) {
		t.Errorf("no file: %+v, %v; want s", k, err)
	}
	cases := []struct {
		name, body string
		want       wire.KeyStroke
		refuse     bool
	}{
		{"moved to x", `{"bindings":[{"context":"ModelPicker","bindings":{"x":"modelPicker:thisSessionOnly","s":null}}]}`,
			wire.KeyStroke{Text: "x"}, false},
		{"moved to ctrl+o", `{"bindings":[{"context":"ModelPicker","bindings":{"ctrl+o":"modelPicker:thisSessionOnly"}}]}`,
			wire.KeyStroke{Code: "KeyO", Mods: wire.ModCtrl}, false},
		{"s unbound", `{"bindings":[{"context":"ModelPicker","bindings":{"s":null}}]}`, wire.KeyStroke{}, true},
		{"s rebound", `{"bindings":[{"context":"ModelPicker","bindings":{"s":"modelPicker:increaseEffort"}}]}`, wire.KeyStroke{}, true},
		{"s in Select", `{"bindings":[{"context":"Select","bindings":{"s":"select:accept"}}]}`, wire.KeyStroke{}, true},
		{"other contexts don't matter", `{"bindings":[{"context":"Chat","bindings":{"s":"chat:submit"}}]}`,
			wire.KeyStroke{Text: "s"}, false},
		{"bad json", `{"bindings":[`, wire.KeyStroke{}, true},
		{"a chord we can't press", `{"bindings":[{"context":"ModelPicker","bindings":{"f5":"modelPicker:thisSessionOnly","s":null}}]}`,
			wire.KeyStroke{}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			write(c.body)
			k, err := sessionOnlyKey(path)
			if c.refuse {
				if err == nil {
					t.Errorf("got %+v, want a refusal", k)
				}
				return
			}
			if err != nil || k != c.want {
				t.Errorf("got %+v, %v; want %+v", k, err, c.want)
			}
		})
	}
}

// --- A scripted picker --------------------------------------------------------

// simModel is one row of the scripted picker, with the effort levels its model
// offers (low → high) and the one the picker shows when the row is focused.
type simModel struct {
	label  string
	levels []string
	def    string
}

// simRows is the 2.1.318 list from the live capture. The effort levels are
// the sim's own, not the models' (live, Haiku 5.5 took xhigh): Opus 4.6
// skips xhigh, which is the overshoot case, Sonnet 4.6 stops at high, which
// is the clamp case, and Haiku 4.5 has no effort line at all, as it had live.
func simRows() []simModel {
	all := []string{"low", "medium", "high", "xhigh", "max"}
	return []simModel{
		{"Default (recommended)", all, "medium"},
		{"Opus 5.5", all, "medium"},
		{"Fable 5.1", all, "high"},
		{"Sonnet 5.5", all, "medium"},
		{"Haiku 5.5", all, "medium"},
		{"Haiku 4.5", nil, ""},
		{"Sonnet 5", all, "medium"},
		{"Opus 5", all, "medium"},
		{"Fable 5", all, "high"},
		{"Opus 4.8", all, "medium"},
		{"Opus 4.7", all, "medium"},
		{"Opus 4.6", []string{"low", "medium", "high", "max"}, "medium"},
		{"Sonnet 4.6", []string{"low", "medium", "high"}, "medium"},
	}
}

// simPicker plays a claude pane with the /model picker: it keeps the list,
// the window, the focus and the effort the way the picker does, draws them
// as 2.1.318 does, and fails the test on the one key this road must never
// press — Enter while the picker is up.
type simPicker struct {
	t       *testing.T
	models  []simModel
	current int // the ✔ row (0-based)
	window  int // visible rows

	// What the pane does on `s`: "session" (default), "saved", "confirm",
	// "hook", "consent".
	outcome       string
	searchFirst   bool // 2.1.283: open in the search box
	noSessionOnly bool // an older picker: no `s` in the footer
	noKeys        bool // a cats server without pane.send_keys

	open, search bool
	focus, top   int
	effort       string
	toggled      bool
	dialog       string // a dialog drawn at the bottom instead of the input box
	transcript   []string
	log          []string
}

func newSim(t *testing.T) *simPicker {
	return &simPicker{t: t, models: simRows(), current: 4, window: 7, outcome: "session"}
}

func (p *simPicker) sendInput(_ uint32, text string, submit bool) error {
	p.log = append(p.log, "input "+text+map[bool]string{true: "⏎"}[submit])
	switch {
	case text == "/model" && submit:
		p.open, p.search = true, p.searchFirst
		p.focus, p.top, p.toggled = p.current, 0, false
		p.scroll()
		p.effort = p.models[p.focus].def
		p.transcript = append(p.transcript, "❯ /model")
	case text == "" && submit:
		if p.open {
			p.t.Fatalf("Enter pressed inside the picker — that saves the default (log %q)", p.log)
		}
		if p.dialog == "confirm" {
			p.dialog = ""
			p.result()
		}
	}
	return nil
}

func (p *simPicker) sendKeys(_ uint32, keys []wire.KeyStroke) error {
	if p.noKeys {
		return errNoSendKeys
	}
	if len(keys) == 0 {
		return errors.New("send_keys: no keys — nothing to send")
	}
	for _, k := range keys {
		name := k.Code
		if k.Text != "" {
			name = k.Text
		}
		p.log = append(p.log, "key "+name)
		if !p.open {
			continue
		}
		if p.search {
			switch name {
			case "ArrowDown":
				p.search = false
			case "Escape":
				p.open = false
			case "s":
				p.t.Fatalf("s typed into the search box (log %q)", p.log)
			}
			continue
		}
		switch name {
		case "ArrowDown":
			p.move(min(p.focus+1, len(p.models)-1))
		case "ArrowUp":
			p.move(max(p.focus-1, 0))
		case "Home":
			p.move(0)
		case "End":
			p.move(len(p.models) - 1)
		case "ArrowLeft", "ArrowRight":
			lv := p.models[p.focus].levels
			at := slices.Index(lv, p.effort)
			if lv == nil || at < 0 {
				continue
			}
			if name == "ArrowLeft" {
				at = max(at-1, 0)
			} else {
				at = min(at+1, len(lv)-1)
			}
			p.effort, p.toggled = lv[at], true
		case "Escape":
			p.open = false
			p.transcript = append(p.transcript, "  ⎿  Kept model as "+p.models[p.current].label)
		case "s":
			if p.noSessionOnly {
				continue
			}
			p.open = false
			switch p.outcome {
			case "session", "saved":
				p.result()
			default:
				p.dialog = p.outcome
			}
		}
	}
	return nil
}

func (p *simPicker) move(to int) {
	p.focus = to
	p.scroll()
	if !p.toggled {
		p.effort = p.models[to].def
	}
}

func (p *simPicker) scroll() {
	if p.focus < p.top {
		p.top = p.focus
	}
	if p.focus >= p.top+p.window {
		p.top = p.focus - p.window + 1
	}
}

// result prints the line `s` (or the confirm's Yes) leaves.
func (p *simPicker) result() {
	m := p.models[p.focus]
	line := "  ⎿  Set model to " + m.label
	if p.outcome == "saved" {
		line += " and saved as your default for new sessions"
	} else {
		line += " for this session only"
	}
	if p.toggled {
		line += " with " + p.effort + " effort"
	}
	p.current = p.focus
	p.transcript = append(p.transcript, line)
}

func (p *simPicker) waitForOutputIn(uint32, string, bool, uint32, time.Duration) (bool, error) {
	return false, nil
}

func (p *simPicker) capture(uint32) (string, error) {
	var b strings.Builder
	b.WriteString(" ▐▛███▛█   Claude Code v2.1.318\n")
	for _, l := range p.transcript {
		b.WriteString(l + "\n")
	}
	switch {
	case p.open:
		b.WriteString("────────\n  Select model\n  Switch between Claude models. Your pick becomes the default for new sessions.\n")
		end := min(p.top+p.window, len(p.models))
		for i := p.top; i < end; i++ {
			mark := " "
			switch {
			case i == p.focus:
				mark = "❯"
			case i == p.top && p.top > 0:
				mark = "↑"
			case i == end-1 && end < len(p.models):
				mark = "↓"
			}
			label := p.models[i].label
			if i == p.current {
				label += " ✔"
			}
			fmt.Fprintf(&b, "  %s %-3s %-22s Description\n", mark, fmt.Sprintf("%d.", i+1), label)
		}
		if end < len(p.models) {
			fmt.Fprintf(&b, "     … +%d models\n", len(p.models)-end)
		}
		if p.models[p.focus].levels == nil {
			b.WriteString("  ○ Effort not supported for " + p.models[p.focus].label + "\n")
		} else {
			word := strings.ToUpper(p.effort[:1]) + p.effort[1:]
			if p.effort == "xhigh" {
				word = "xHigh"
			}
			b.WriteString("  ◐ " + word + " effort ←/→ to adjust\n")
		}
		switch {
		case p.search:
			b.WriteString("  Type to search · Enter to set as default · ↓ to list · Esc to cancel\n")
		case p.noSessionOnly:
			b.WriteString("  Enter to confirm · Esc to cancel\n")
		default:
			b.WriteString("  Enter to set as default · s to use this session only · Esc to cancel\n")
		}
	case p.dialog == "confirm":
		b.WriteString("  Switch model?\n  Your next response will be slower\n  ❯ 1. Yes, switch to X\n    2. No, go back\n")
	case p.dialog == "hook":
		b.WriteString("  Switch model?\n  A PreModelSwitch hook asked you to confirm\n  ❯ 1. Yes, switch to X\n    2. No, go back\n")
	case p.dialog == "consent":
		b.WriteString("  Switch to Fable?\n  Fable uses usage credits\n  ❯ 1. Yes\n")
	default:
		b.WriteString("────────\n❯ \n────────\n  ? for shortcuts\n")
	}
	return b.String(), nil
}

// keys is the log's key presses only, for asserting what was pressed.
func (p *simPicker) keys() []string {
	var out []string
	for _, l := range p.log {
		if k, ok := strings.CutPrefix(l, "key "); ok {
			out = append(out, k)
		}
	}
	return out
}

// fastPicker shrinks the waits and points keybindings.json at an empty dir.
func fastPicker(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	for _, w := range []*time.Duration{&pickerOpenWait, &pickerStepWait, &pickerOutcomeWait} {
		old := *w
		*w = 150 * time.Millisecond
		t.Cleanup(func() { *w = old })
	}
}

// TestPickSessionOnlyModelAndEffort is N-079's main road: the target row is
// below the window, so the list is read past it; the effort walks down one
// press at a time; `s` applies both, and the note is the pane's own words.
func TestPickSessionOnlyModelAndEffort(t *testing.T) {
	fastPicker(t)
	p := newSim(t)
	note, err := pickSessionOnly(p, 1, "claude-sonnet-4-6", "low", 0)
	if err != nil {
		t.Fatalf("err = %v (log %q)", err, p.log)
	}
	if p.current != 12 || p.effort != "low" {
		t.Errorf("pane on %s at %s, want Sonnet 4.6 at low", p.models[p.current].label, p.effort)
	}
	if want := "set Sonnet 4.6 for this session only with low effort"; note != want {
		t.Errorf("note = %q, want %q", note, want)
	}
	keys := p.keys()
	if keys[len(keys)-1] != "s" || slices.Contains(keys, "Enter") {
		t.Errorf("keys = %q, want them to end on s and never press Enter", keys)
	}
}

// TestPickSessionOnlyAlias: an alias lands on its family's first row, which
// is visible, and with no effort asked the effort keys are never touched.
func TestPickSessionOnlyAlias(t *testing.T) {
	fastPicker(t)
	p := newSim(t)
	if _, err := pickSessionOnly(p, 1, "sonnet", "", 0); err != nil {
		t.Fatal(err)
	}
	if got := p.models[p.current].label; got != "Sonnet 5.5" {
		t.Errorf("picked %s, want Sonnet 5.5", got)
	}
	for _, k := range p.keys() {
		if k == "ArrowLeft" || k == "ArrowRight" {
			t.Errorf("effort keys pressed with no effort asked: %q", p.keys())
		}
	}
}

// TestPickSessionOnlyEffortOnly stays on the row the picker opens on (the
// pane's model) and only walks the effort.
func TestPickSessionOnlyEffortOnly(t *testing.T) {
	fastPicker(t)
	p := newSim(t)
	note, err := pickSessionOnly(p, 1, "", "high", 0)
	if err != nil {
		t.Fatal(err)
	}
	// The empty send_keys probe is refused before it is logged, so these
	// are all the keys the pane saw: no Home, no ↑/↓.
	if want := []string{"ArrowRight", "s"}; !slices.Equal(p.keys(), want) {
		t.Errorf("keys = %q, want medium → high and s", p.keys())
	}
	if !strings.Contains(note, "Haiku 5.5 for this session only with high effort") {
		t.Errorf("note = %q", note)
	}
}

// TestPickSessionOnlyRefusals: every road that can't be sure cancels the
// picker with Esc (which saves nothing), says why, and never presses `s`.
func TestPickSessionOnlyRefusals(t *testing.T) {
	cases := []struct {
		name          string
		setup         func(*simPicker)
		model, effort string
		want          string
	}{
		{"a level the model skips (overshoot)", nil, "claude-opus-4-6", "xhigh", "Opus 4.6 has no xhigh effort"},
		{"a level past the model's top (clamp)", nil, "claude-sonnet-4-6", "max", "Sonnet 4.6 has no max effort"},
		{"a model with no effort", nil, "claude-haiku-4-5", "low", "Haiku 4.5 takes no effort setting"},
		{"a model with no row", nil, "opus[1m]", "", `no row for "opus[1m]"`},
		{"an older picker", func(p *simPicker) { p.noSessionOnly = true }, "opus", "", "offers no session-only pick"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fastPicker(t)
			p := newSim(t)
			if c.setup != nil {
				c.setup(p)
			}
			_, err := pickSessionOnly(p, 1, c.model, c.effort, 0)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want it to contain %q", err, c.want)
			}
			if p.open {
				t.Errorf("picker left open: %q", p.log)
			}
			if p.current != 4 || slices.Contains(p.keys(), "s") {
				t.Errorf("the pick was made anyway: %q", p.keys())
			}
			if !strings.Contains(err.Error(), "prompt was not sent") {
				t.Errorf("err = %v, want it to say the prompt was not sent", err)
			}
		})
	}
}

// TestPickSessionOnlySearchMode: 2.1.283 opened the picker in a search box,
// where `s` is a filter letter. ↓ leaves it before anything else.
func TestPickSessionOnlySearchMode(t *testing.T) {
	fastPicker(t)
	p := newSim(t)
	p.searchFirst = true
	if _, err := pickSessionOnly(p, 1, "opus", "", 0); err != nil {
		t.Fatalf("err = %v (log %q)", err, p.log)
	}
	if k := p.keys(); k[0] != "ArrowDown" {
		t.Errorf("first key = %q, want ↓ out of the search box", k[0])
	}
	if got := p.models[p.current].label; got != "Opus 5.5" {
		t.Errorf("picked %s, want Opus 5.5", got)
	}
}

// TestPickSessionOnlyOutcomes: what the pane says after `s` decides the drop.
func TestPickSessionOnlyOutcomes(t *testing.T) {
	cases := []struct {
		outcome  string
		wantErr  string
		wantNote string
		enter    bool
	}{
		{"saved", "saved as your default", "", false},
		{"confirm", "", "(confirmed the switch", true},
		{"hook", "PreModelSwitch hook", "", false},
		{"consent", "usage credits", "", false},
	}
	for _, c := range cases {
		t.Run(c.outcome, func(t *testing.T) {
			fastPicker(t)
			p := newSim(t)
			p.outcome = c.outcome
			note, err := pickSessionOnly(p, 1, "opus", "", 0)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want %q", err, c.wantErr)
				}
			} else if err != nil || !strings.Contains(note, c.wantNote) {
				t.Fatalf("note, err = %q, %v; want note with %q", note, err, c.wantNote)
			}
			if enter := slices.Contains(p.log, "input ⏎"); enter != c.enter {
				t.Errorf("bare Enter sent = %v, want %v (log %q)", enter, c.enter, p.log)
			}
		})
	}
}

// TestPickSessionOnlyNeedsSendKeys: on a cats server without pane.send_keys
// the picker could be opened but never steered or closed, so /model is not
// even sent.
func TestPickSessionOnlyNeedsSendKeys(t *testing.T) {
	fastPicker(t)
	p := newSim(t)
	p.noKeys = true
	_, err := pickSessionOnly(p, 1, "opus", "low", 0)
	if err == nil || !strings.Contains(err.Error(), "pane.send_keys") {
		t.Fatalf("err = %v, want the missing verb named", err)
	}
	if len(p.log) != 0 {
		t.Errorf("sent %q to a pane it can't steer", p.log)
	}
}

// TestPickSessionOnlyReadsBelowTheEcho: an earlier drop's result line above
// this /model is not this one's outcome.
func TestPickSessionOnlyReadsBelowTheEcho(t *testing.T) {
	screen := "❯ /model\n  ⎿  Set model to Opus 5.5 for this session only\n⏺ done\n❯ /model\n────\n❯ \n"
	if out, _ := readOutcome(screen); out != outcomePending {
		t.Errorf("outcome = %v, want pending: the result is the earlier drop's", out)
	}
	wrapped := "❯ /model\n  ⎿  Set model to Sonnet 4.6 for this\n     session only with low effort\n────\n❯ \n"
	if out, line := readOutcome(wrapped); out != outcomeSession || line != "Set model to Sonnet 4.6 for this session only with low effort" {
		t.Errorf("wrapped result = %v %q", out, line)
	}
}
