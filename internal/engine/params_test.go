package engine

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hhx465453939/stemscribe/internal/registry"
)

// TestSpecsCoverage is AC-01: every declared spec must be readable through
// WhisperParams. A spec the struct cannot render would appear in the UI and do
// nothing, which is worse than not showing it.
func TestSpecsCoverage(t *testing.T) {
	p := Defaults()
	for _, s := range Specs() {
		if s.Key == "" || s.Flag == "" || s.Group == "" {
			t.Errorf("spec %+v is missing key/flag/group", s)
			continue
		}
		if !strings.HasPrefix(s.Flag, "-") {
			t.Errorf("spec %s: flag %q must start with a dash (it comes from --help)", s.Key, s.Flag)
		}
		if _, ok := p.valueByKey(s.Key); !ok {
			t.Errorf("spec %q has no field mapping in WhisperParams.valueByKey", s.Key)
		}
		if s.Help == "" {
			t.Errorf("spec %q has no help text; the UI would show an unexplained control", s.Key)
		}
	}
}

// TestSpecsUnique guards against a copy-paste key or flag collision.
func TestSpecsUnique(t *testing.T) {
	keys := map[string]bool{}
	flags := map[string]bool{}
	for _, s := range Specs() {
		if keys[s.Key] {
			t.Errorf("duplicate spec key %q", s.Key)
		}
		keys[s.Key] = true
		if flags[s.Flag] {
			t.Errorf("duplicate flag %q (key %s)", s.Flag, s.Key)
		}
		flags[s.Flag] = true
	}
}

// TestSpecsGroupsCovered ensures Groups() offers a bucket for every spec, because
// the UI renders by iterating Groups().
func TestSpecsGroupsCovered(t *testing.T) {
	known := map[string]bool{}
	for _, g := range Groups() {
		if known[g] {
			t.Errorf("group %q listed twice", g)
		}
		known[g] = true
	}
	for _, s := range Specs() {
		if !known[s.Group] {
			t.Errorf("spec %q uses group %q which Groups() does not list", s.Key, s.Group)
		}
	}
}

// TestDefaultsMatchSpecs is AC-02 at the unit level: the shipped Defaults() must
// equal the values the binary reports, otherwise the UI shows a number the engine
// does not actually use.
//
// Comparison is by type rather than by formatted string: hand-formatting floats
// would make 2.40 vs 2.4 look like a mismatch when the values are identical.
func TestDefaultsMatchSpecs(t *testing.T) {
	p := Defaults()
	for _, s := range Specs() {
		raw, ok := p.valueByKey(s.Key)
		if !ok {
			continue
		}
		switch s.Kind {
		case KindFloat:
			want, wok := s.Def.(float64)
			if !wok {
				t.Errorf("%s: float spec has non-float default %v", s.Key, s.Def)
				continue
			}
			var got float64
			parsed, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				t.Errorf("%s: cannot read %q as float: %v", s.Key, raw, err)
				continue
			}
			got = parsed
			if got != want {
				t.Errorf("default mismatch for %s (%s): got %v, spec %v", s.Key, s.Flag, got, want)
			}
		case KindInt:
			want, wok := s.Def.(int)
			if !wok {
				t.Errorf("%s: int spec has non-int default %v", s.Key, s.Def)
				continue
			}
			var got int
			parsedInt, err := strconv.Atoi(raw)
			if err != nil {
				t.Errorf("%s: cannot read %q as int: %v", s.Key, raw, err)
				continue
			}
			got = parsedInt
			if got != want {
				t.Errorf("default mismatch for %s (%s): got %d, spec %d", s.Key, s.Flag, got, want)
			}
		case KindBool:
			want, wok := s.Def.(bool)
			if !wok {
				t.Errorf("%s: bool spec has non-bool default %v", s.Key, s.Def)
				continue
			}
			got := raw == "true"
			if got != want {
				t.Errorf("default mismatch for %s (%s): got %v, spec %v", s.Key, s.Flag, got, want)
			}
		default:
			want, _ := s.Def.(string)
			if raw != want {
				t.Errorf("default mismatch for %s (%s): got %q, spec %q", s.Key, s.Flag, raw, want)
			}
		}
	}
}

// TestArgsLanguageIsThreeDimensions is AC-03: the three language controls are
// independent flags, not one merged dropdown.
func TestArgsLanguageIsThreeDimensions(t *testing.T) {
	zh := Defaults()
	zh.InputLanguage = "zh"
	if !testHasFlagValue(zh.Args(), "-l", "zh") {
		t.Errorf("input_language=zh must emit -l zh, got %v", zh.Args())
	}

	tr := Defaults()
	tr.Translate = true
	if !testHasArg(tr.Args(), "-tr") {
		t.Errorf("translate must emit -tr, got %v", tr.Args())
	}

	dl := Defaults()
	dl.DetectOnly = true
	if !testHasArg(dl.Args(), "-dl") {
		t.Errorf("detect_only must emit -dl, got %v", dl.Args())
	}

	// English is the binary default, so emitting it would be noise.
	en := Defaults()
	if testHasArg(en.Args(), "-l") {
		t.Errorf("default language must not be emitted, got %v", en.Args())
	}
}

// TestArgsQualityKnobs is AC-03 for the quality group.
func TestArgsQualityKnobs(t *testing.T) {
	p := Defaults()
	p.BeamSize = 1
	p.NoFallback = true
	p.FlashAttn = false
	args := p.Args()

	if !testHasFlagValue(args, "-bs", "1") {
		t.Errorf("beam_size=1 must emit -bs 1, got %v", args)
	}
	if !testHasArg(args, "-nf") {
		t.Errorf("no_fallback must emit -nf, got %v", args)
	}
	// -fa defaults to true, so disabling it needs the negative form only.
	if !testHasArg(args, "-nfa") {
		t.Errorf("flash_attn=false must emit -nfa, got %v", args)
	}
	if testHasArg(args, "-fa") {
		t.Errorf("flash_attn=false must not also emit -fa, got %v", args)
	}
}

// TestArgsJSONAlwaysOn: the pipeline parses whisper's JSON, so it cannot be off.
func TestArgsJSONAlwaysOn(t *testing.T) {
	p := Defaults()
	p.OutJSON = false
	if !testHasArg(p.Args(), "-oj") {
		t.Errorf("JSON output is required by the pipeline, got %v", p.Args())
	}
}

// TestValidateConflicts is AC-05: combinations that would otherwise fail silently
// must be refused up front with a hint the user can act on.
func TestValidateConflicts(t *testing.T) {
	multi := registry.Model{ID: "whisper-tiny", DisplayName: "Whisper Tiny"}
	english := registry.Model{ID: "whisper-tiny.en", DisplayName: "Whisper Tiny English"}

	cases := []struct {
		name  string
		p     WhisperParams
		model registry.Model
		key   string
	}{
		{"en model + zh", WhisperParams{InputLanguage: "zh"}, english, "input_language"},
		{"en model + auto", WhisperParams{InputLanguage: "auto"}, english, "input_language"},
		{"detect + translate", WhisperParams{InputLanguage: "zh", DetectOnly: true, Translate: true}, multi, "translate"},
		{"vad without model", WhisperParams{InputLanguage: "zh", VAD: true}, multi, "vad"},
		{"temperature out of range", WhisperParams{InputLanguage: "zh", Temperature: 1.5}, multi, "temperature"},
		{"negative threads", WhisperParams{InputLanguage: "zh", Threads: -1}, multi, "quality"},
	}
	for _, c := range cases {
		err := c.p.Validate(c.model)
		if err == nil {
			t.Errorf("%s: expected a conflict, got nil", c.name)
			continue
		}
		conflict, ok := err.(*ErrParamConflict)
		if !ok {
			t.Errorf("%s: error should be *ErrParamConflict, got %T", c.name, err)
			continue
		}
		if conflict.Key != c.key {
			t.Errorf("%s: conflict key = %q, want %q", c.name, conflict.Key, c.key)
		}
		if conflict.Hint == "" {
			t.Errorf("%s: conflict must carry an actionable hint", c.name)
		}
	}
}

// TestValidateAcceptsValid confirms the guards do not reject normal use.
func TestValidateAcceptsValid(t *testing.T) {
	multi := registry.Model{ID: "whisper-tiny", DisplayName: "Whisper Tiny"}
	ok := []WhisperParams{
		{InputLanguage: "zh"},
		{InputLanguage: "auto"},
		{InputLanguage: "auto", Translate: true},
		{InputLanguage: "en"},
		{InputLanguage: "zh", VAD: true, VADModelPath: "vad.bin"},
	}
	for i, p := range ok {
		if err := p.Validate(multi); err != nil {
			t.Errorf("case %d should validate: %v", i, err)
		}
	}
	// An .en model with English explicitly set is legitimate.
	if err := (WhisperParams{InputLanguage: "en"}).Validate(registry.Model{ID: "whisper-tiny.en"}); err != nil {
		t.Errorf(".en model with en must validate: %v", err)
	}
}

// TestSnapshotOnlyDeviations is AC-04's unit half: provenance records what the run
// changed, not thirty defaults.
func TestSnapshotOnlyDeviations(t *testing.T) {
	p := Defaults()
	if got := p.Snapshot(); len(got) != 0 {
		t.Errorf("default parameter set should produce an empty snapshot, got %v", got)
	}

	p.InputLanguage = "zh"
	p.BeamSize = 1
	snap := p.Snapshot()
	if snap["input_language"] != "zh" {
		t.Errorf("snapshot missing input_language, got %v", snap)
	}
	if snap["beam_size"] != "1" {
		t.Errorf("snapshot missing beam_size, got %v", snap)
	}
	if _, exists := snap["threads"]; exists {
		t.Errorf("snapshot must not record unchanged defaults, got %v", snap)
	}
}

// TestHashInputChangesWithParams is AC-04's invalidation half.
func TestHashInputChangesWithParams(t *testing.T) {
	base := Defaults().HashInput()

	changed := Defaults()
	changed.BeamSize = 1
	if changed.HashInput() == base {
		t.Error("changing beam_size must change the stage hash, otherwise the transcript would not re-run")
	}

	same := Defaults()
	same.BeamSize = 1
	if same.HashInput() != changed.HashInput() {
		t.Error("equal parameters must hash equally, otherwise resume always re-runs")
	}

	// Field assignment order must not matter for equal content.
	a := Defaults()
	a.InputLanguage = "zh"
	a.BeamSize = 1
	b := Defaults()
	b.BeamSize = 1
	b.InputLanguage = "zh"
	if a.HashInput() != b.HashInput() {
		t.Error("hash must be order independent")
	}
}

// testHasArg / testHasFlagValue are local to this file: whisper_test.go already
// defines hasArg, and Go has one package-wide namespace per test binary.
func testHasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func testHasFlagValue(args []string, flag, want string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == want {
			return true
		}
	}
	return false
}
