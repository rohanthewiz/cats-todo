package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rohanthewiz/cats/wire"
)

// strokesText reads a pane.send_keys call back as text: typed runs as they are,
// shift+Enter as "\n", a bare Enter as "⏎", any other named key as <Code>.
func strokesText(keys []wire.KeyStroke) string {
	var b strings.Builder
	for _, k := range keys {
		switch {
		case k.Text != "":
			b.WriteString(k.Text)
		case k == shiftEnter:
			b.WriteString("\n")
		case k.Code == "Enter" && k.Mods == 0:
			b.WriteString("⏎")
		default:
			b.WriteString("<" + k.Code + ">")
		}
	}
	return b.String()
}

// TestTypesKeys pins the rule typing.go's table states: prose into a claude
// pane is typed; a slash command, a leading "!" (bash mode), a control
// character, another agent or nothing at all is not.
func TestTypesKeys(t *testing.T) {
	cases := []struct {
		name, agent, text string
		want              bool
	}{
		{"prose into claude", "claude", "fix the flaky test", true},
		{"a launch path is still claude", "/usr/local/bin/claude --model opus", "fix it", true},
		{"newlines, tabs and non-ASCII type", "claude", "one\r\ntwo\tthree — é ✓", true},
		{"a slash later is prose", "claude", "run /code-review", true},
		{"slash command", "claude", "/sess-load 2", false},
		{"indented slash command", "claude", "  /compact", false},
		{"bash mode", "claude", "!rm -rf build", false},
		{"bash mode after blank lines", "claude", "\n\n!ls", false},
		{"escape byte", "claude", "colour \x1b[31m", false},
		{"invalid UTF-8", "claude", "bad \xff", false},
		{"empty", "claude", "", false},
		{"only whitespace", "claude", " \n ", false},
		{"another agent", "codex", "fix the flaky test", false},
		{"no agent", "", "fix the flaky test", false},
	}
	for _, c := range cases {
		if got := typesKeys(c.agent, c.text); got != c.want {
			t.Errorf("%s: typesKeys(%q, %q) = %v, want %v", c.name, c.agent, c.text, got, c.want)
		}
	}
}

// TestTypeBatches pins the chunking: the calls read back as the text (line
// breaks as shift+Enter, CRLF and CR folded, tabs as spaces), no call is empty,
// and none carries more than typeChunk keys — the bound that keeps Claude Code
// from taking the burst for a paste.
func TestTypeBatches(t *testing.T) {
	if got := typeBatches("ab\ncd"); len(got) != 1 || strokesText(got[0]) != "ab\ncd" ||
		!slices.Equal(got[0], []wire.KeyStroke{{Text: "ab"}, shiftEnter, {Text: "cd"}}) {
		t.Fatalf("ab\\ncd = %+v", got)
	}
	if got := typeBatches("a\r\nb\rc\td"); len(got) != 1 || strokesText(got[0]) != "a\nb\nc    d" {
		t.Fatalf("CRLF/CR/tab = %q", strokesText(got[0]))
	}

	long := strings.Repeat("word — é ", 40) + "\n\n" + strings.Repeat("x", 3*typeChunk+5) + "\nend"
	batches := typeBatches(long)
	var back strings.Builder
	for i, b := range batches {
		if len(b) == 0 {
			t.Fatalf("call %d is empty", i)
		}
		keys := 0
		for _, k := range b {
			if k.Text != "" {
				keys += utf8.RuneCountInString(k.Text)
			} else {
				keys++
			}
		}
		if keys > typeChunk {
			t.Fatalf("call %d carries %d keys, more than typeChunk (%d)", i, keys, typeChunk)
		}
		back.WriteString(strokesText(b))
	}
	if back.String() != long {
		t.Fatalf("batches read back as\n%q\nwant\n%q", back.String(), long)
	}
}

// keysPane is a paneDriver for the typed road: it logs pastes and key calls,
// and can play a cats server older than pane.send_keys, or one that fails on
// the nth key call.
type keysPane struct {
	calls  []string
	noKeys bool
	failAt int // 1-based key call that fails; 0 = none
	nkeys  int
}

func (p *keysPane) sendInput(_ uint32, text string, submit bool) error {
	p.calls = append(p.calls, "paste "+text+map[bool]string{true: " ⏎", false: ""}[submit])
	return nil
}

func (p *keysPane) sendKeys(_ uint32, keys []wire.KeyStroke) error {
	if p.noKeys {
		return errNoSendKeys
	}
	p.nkeys++
	if p.nkeys == p.failAt {
		return errors.New("pane 7 has exited")
	}
	p.calls = append(p.calls, "keys "+strokesText(keys))
	return nil
}

func (p *keysPane) capture(uint32) (string, error) { return "", nil }

func (p *keysPane) waitForOutputIn(uint32, string, bool, uint32, time.Duration) (bool, error) {
	p.calls = append(p.calls, "wait")
	return true, nil
}

// TestDeliverPromptTyped drives deliverPrompt's typed road (N-072): a prose
// prompt into a claude pane is typed with no pasteAsk, then submitted with an
// Enter of its own; with images there is no wait (a typed path is not
// attached, so there is nothing for the Enter to race); paste mode presses no
// Enter; and a cats server without pane.send_keys gets the old paste, lead and
// all, with nothing typed first.
func TestDeliverPromptTyped(t *testing.T) {
	p := &keysPane{}
	if err := deliverPrompt(p, 7, "claude", "fix it\nnow", true, true); err != nil {
		t.Fatal(err)
	}
	if want := []string{"keys fix it\nnow", "keys ⏎"}; !slices.Equal(p.calls, want) {
		t.Errorf("run = %q, want %q", p.calls, want)
	}

	p = &keysPane{}
	if err := deliverPrompt(p, 7, "claude", "fix it", false, false); err != nil {
		t.Fatal(err)
	}
	if want := []string{"keys fix it"}; !slices.Equal(p.calls, want) {
		t.Errorf("paste mode = %q, want %q", p.calls, want)
	}

	p = &keysPane{noKeys: true}
	if err := deliverPrompt(p, 7, "claude", "fix it", true, false); err != nil {
		t.Fatal(err)
	}
	if want := []string{"paste " + withPasteAsk("fix it") + " ⏎"}; !slices.Equal(p.calls, want) {
		t.Errorf("old cats = %q, want %q", p.calls, want)
	}

	p = &keysPane{}
	if err := deliverPrompt(p, 7, "claude", "!make test", true, false); err != nil {
		t.Fatal(err)
	}
	if want := []string{"paste " + withPasteAsk("!make test") + " ⏎"}; !slices.Equal(p.calls, want) {
		t.Errorf("bang prompt = %q, want %q", p.calls, want)
	}
}

// A key call failing after the first is not "cats is too old": part of the
// prompt is in the pane, so the drop fails and says so, and nothing is pasted
// on top of it.
func TestDeliverPromptTypedFailsPartWay(t *testing.T) {
	p := &keysPane{failAt: 2}
	err := deliverPrompt(p, 7, "claude", strings.Repeat("y", 3*typeChunk), true, false)
	if err == nil || !strings.Contains(err.Error(), "part-way (1 of 3 chunks sent)") {
		t.Fatalf("err = %v, want a part-way failure", err)
	}
	for _, c := range p.calls {
		if strings.HasPrefix(c, "paste") {
			t.Fatalf("a part-way failure must not paste the prompt again: %q", p.calls)
		}
	}
}

// deliverLine is a loop's between command: typed when it is prose, pasted bare
// (no lead) when it is a slash command or cats cannot type.
func TestDeliverLine(t *testing.T) {
	p := &keysPane{}
	_ = deliverLine(p, 7, "claude", "/compact")
	_ = deliverLine(p, 7, "claude", "keep going")
	if want := []string{"paste /compact ⏎", "keys keep going", "keys ⏎"}; !slices.Equal(p.calls, want) {
		t.Errorf("calls = %q, want %q", p.calls, want)
	}
	p = &keysPane{noKeys: true}
	_ = deliverLine(p, 7, "claude", "keep going")
	if want := []string{"paste keep going ⏎"}; !slices.Equal(p.calls, want) {
		t.Errorf("old cats = %q, want %q", p.calls, want)
	}
}
